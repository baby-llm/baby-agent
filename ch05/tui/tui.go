package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"babyagent/ch05"
)

type streamMsg struct {
	event ch05.MessageVO
}

type streamClosedMsg struct{}

type streamDoneMsg struct {
	err error
}

type runState int

const (
	stateIdle runState = iota
	stateRunning
	stateAborting
)

type activeStream struct {
	events <-chan ch05.MessageVO
	cancel context.CancelFunc

	turnLogLen   int
	reasonBody   int
	contentBody  int
	policyBody   int // 当前策略 log entry 的索引
	streamClosed bool
	doneReceived bool
	doneErr      error
}

type TuiViewModel struct {
	modelName string
	agent     *ch05.Agent

	input string
	logs  []LogEntry

	state  runState
	active *activeStream

	notice string

	width  int
	height int

	logsViewport viewport.Model
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	labelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	noticeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	borderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)

func NewModel(agent *ch05.Agent, modelName string) *TuiViewModel {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.MouseWheelEnabled = false

	return &TuiViewModel{
		modelName:    modelName,
		agent:        agent,
		logs:         make([]LogEntry, 0),
		logsViewport: vp,
	}
}

func (m *TuiViewModel) Init() tea.Cmd {
	return nil
}

func waitStreamEvent(ch <-chan ch05.MessageVO) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return streamMsg{event: msg}
	}
}

func waitStreamDone(ch <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return streamDoneMsg{}
		}
		return streamDoneMsg{err: err}
	}
}

func (m *TuiViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncLogsViewportSize()
		return m, nil
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollUp(3)
		case tea.MouseWheelDown:
			m.scrollDown(3)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.PasteMsg:
		// Handle paste from clipboard
		if m.state == stateIdle {
			m.input += msg.Content
		}
		return m, nil
	case streamMsg:
		return m.handleStreamMsg(msg)
	case streamClosedMsg:
		if m.active != nil {
			m.active.events = nil
			m.active.streamClosed = true
			if m.active.doneReceived {
				return m.finalizeActiveStream()
			}
		}
		return m, nil
	case streamDoneMsg:
		return m.handleStreamDone(msg)
	}
	return m, nil
}

func (m *TuiViewModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.stopActiveStream()
		return m, tea.Quit
	case "up":
		m.scrollUp(1)
		return m, nil
	case "down":
		m.scrollDown(1)
		return m, nil
	case "pgup":
		m.scrollUp(m.logsViewportHeight())
		return m, nil
	case "pgdown":
		m.scrollDown(m.logsViewportHeight())
		return m, nil
	case "home":
		m.logsViewport.GotoTop()
		return m, nil
	case "end":
		m.logsViewport.GotoBottom()
		return m, nil
	case "enter":
		return m.handleSubmit()
	case "esc":
		m.abortCurrentTurn()
		return m, nil
	case "backspace":
		if len(m.input) > 0 {
			r := []rune(m.input)
			m.input = string(r[:len(r)-1])
		}
		return m, nil
	}

	if m.state != stateIdle {
		return m, nil
	}

	if key := msg.Key(); key.Text != "" {
		m.input += key.Text
	}
	return m, nil
}

func (m *TuiViewModel) handleSubmit() (tea.Model, tea.Cmd) {
	query := strings.TrimSpace(m.input)
	if query == "" {
		return m, nil
	}

	if m.state != stateIdle {
		return m, nil
	}

	m.input = ""
	if query == "/clear" {
		m.clearSession()
		return m, nil
	}

	return m.startNewTurn(query)
}

func (m *TuiViewModel) handleStreamEvent(event ch05.MessageVO) {
	if m.active == nil || m.state == stateAborting {
		return
	}

	switch event.Type {
	case ch05.MessageTypeReasoning:
		if event.ReasoningContent == nil {
			return
		}
		if m.active.reasonBody == -1 {
			m.logs = append(m.logs, NewReasoning(*event.ReasoningContent))
			m.active.reasonBody = len(m.logs) - 1
		} else {
			m.logs[m.active.reasonBody].AppendContent(*event.ReasoningContent)
		}
	case ch05.MessageTypeContent:
		if event.Content == nil {
			return
		}
		if m.active.contentBody == -1 {
			m.logs = append(m.logs, NewAnswer(*event.Content))
			m.active.contentBody = len(m.logs) - 1
		} else {
			m.logs[m.active.contentBody].AppendContent(*event.Content)
		}
	case ch05.MessageTypeToolCall:
		if event.ToolCall == nil {
			return
		}
		m.logs = append(m.logs, NewTool(fmt.Sprintf("%s(%s)", event.ToolCall.Name, event.ToolCall.Arguments)))
		m.resetOutputSection()
	case ch05.MessageTypeError:
		if event.Content == nil {
			return
		}
		m.logs = append(m.logs, NewError(*event.Content))
		m.resetOutputSection()
	case ch05.MessageTypePolicy:
		if event.Policy == nil {
			return
		}
		if event.Policy.Running {
			// 策略开始：添加新的 log entry
			m.logs = append(m.logs, NewPolicyRunning(event.Policy.Name))
			m.active.policyBody = len(m.logs) - 1
		} else {
			// 策略结束：更新对应的 log entry
			if m.active.policyBody >= 0 && m.active.policyBody < len(m.logs) {
				m.logs[m.active.policyBody].UpdatePolicyCompleted(event.Policy.Error == nil)
			}
			m.active.policyBody = -1
		}
		m.refreshLogsViewportContent()
	}
}

func (m *TuiViewModel) resetOutputSection() {
	if m.active == nil {
		return
	}
	m.active.reasonBody = -1
	m.active.contentBody = -1
	// 注意：不重置 policyBody，因为策略状态需要保留
}

func (m *TuiViewModel) handleStreamMsg(msg streamMsg) (tea.Model, tea.Cmd) {
	if m.active == nil || m.active.events == nil {
		return m, nil
	}
	m.handleStreamEvent(msg.event)
	m.refreshLogsViewportContent()
	return m, waitStreamEvent(m.active.events)
}

func (m *TuiViewModel) handleStreamDone(msg streamDoneMsg) (tea.Model, tea.Cmd) {
	if m.active == nil {
		m.state = stateIdle
		return m, nil
	}

	m.active.doneReceived = true
	m.active.doneErr = msg.err
	if !m.active.streamClosed {
		return m, nil
	}

	return m.finalizeActiveStream()
}

func (m *TuiViewModel) startNewTurn(query string) (tea.Model, tea.Cmd) {
	m.notice = ""
	turnStart := len(m.logs)
	m.logs = append(m.logs, NewContent(query))

	streamC := make(chan ch05.MessageVO)
	doneC := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	m.active = &activeStream{
		events:      streamC,
		cancel:      cancel,
		turnLogLen:  turnStart,
		reasonBody:  -1,
		contentBody: -1,
		policyBody:  -1,
	}
	m.state = stateRunning
	m.refreshLogsViewportContent()

	go func() {
		err := m.agent.RunStreaming(ctx, query, streamC)
		close(streamC)
		doneC <- err
		close(doneC)
	}()

	return m, tea.Batch(waitStreamEvent(streamC), waitStreamDone(doneC))
}

func (m *TuiViewModel) clearSession() {
	m.agent.ResetSession()
	m.logs = m.logs[:0]
	m.notice = "会话已清空（仅保留 system prompt）。"
	m.refreshLogsViewportContent()
}

func (m *TuiViewModel) abortCurrentTurn() {
	if m.state != stateRunning || m.active == nil || m.active.cancel == nil {
		return
	}
	m.state = stateAborting
	m.notice = "正在终止流式输出..."
	m.active.cancel()
}

func (m *TuiViewModel) rollbackTurn() {
	if m.active == nil {
		return
	}
	if m.active.turnLogLen >= 0 && m.active.turnLogLen <= len(m.logs) {
		m.logs = m.logs[:m.active.turnLogLen]
	}
	m.refreshLogsViewportContent()
}

func (m *TuiViewModel) stopActiveStream() {
	if m.active == nil {
		return
	}
	if m.active.cancel != nil {
		m.active.cancel()
	}
	m.active = nil
}

func (m *TuiViewModel) finalizeActiveStream() (tea.Model, tea.Cmd) {
	if m.active == nil {
		m.state = stateIdle
		return m, nil
	}

	err := m.active.doneErr
	if m.state == stateAborting {
		m.rollbackTurn()
		m.notice = "已取消本轮输入。"
		m.stopActiveStream()
		m.state = stateIdle
		return m, nil
	}

	m.stopActiveStream()
	if err != nil {
		m.logs = append(m.logs, NewError(err.Error()))
	}
	m.logs = append(m.logs, NewBorder())
	m.state = stateIdle
	m.refreshLogsViewportContent()
	return m, nil
}

func (m *TuiViewModel) scrollUp(n int) {
	if n <= 0 {
		return
	}
	m.logsViewport.ScrollUp(n)
}

func (m *TuiViewModel) scrollDown(n int) {
	if n <= 0 {
		return
	}
	m.logsViewport.ScrollDown(n)
}

func (m *TuiViewModel) logsHeaderHeight() int {
	return 4
}

func (m *TuiViewModel) logsFooterHeight() int {
	h := 4
	if m.state != stateIdle {
		h++
	}
	if m.notice != "" {
		h++
	}
	return h
}

func (m *TuiViewModel) logsViewportHeight() int {
	if m.height <= 0 {
		return 1
	}
	h := m.height - m.logsHeaderHeight() - m.logsFooterHeight()
	if h < 1 {
		return 1
	}
	return h
}

func (m *TuiViewModel) syncLogsViewportSize() {
	w := m.width
	if w < 1 {
		w = 1
	}
	m.logsViewport.SetWidth(w)
	m.logsViewport.SetHeight(m.logsViewportHeight())
}

func (m *TuiViewModel) refreshLogsViewportContent() {
	atBottom := m.logsViewport.AtBottom()
	offset := m.logsViewport.YOffset()
	lines := make([]string, len(m.logs))
	for i, entry := range m.logs {
		lines[i] = entry.Render()
	}
	m.logsViewport.SetContent(strings.Join(lines, "\n\n"))
	if !atBottom {
		m.logsViewport.GotoBottom()
		return
	}
	m.logsViewport.SetYOffset(offset)
}

func (m *TuiViewModel) View() tea.View {
	var b strings.Builder

	m.syncLogsViewportSize()

	b.WriteString(titleStyle.Render("BabyAgent TUI (Bubble Tea)"))
	b.WriteString("\n")
	b.WriteString(borderStyle.Render(strings.Repeat("─", 48)))
	b.WriteString("\n")
	b.WriteString(contentStyle.Render("欢迎使用，输入问题后回车。"))
	b.WriteString(labelStyle.Render("当前模型: "))
	b.WriteString(contentStyle.Render(m.modelName))
	b.WriteString("\n")
	b.WriteString(m.logsViewport.View())

	b.WriteString("\n")
	if m.state != stateIdle {
		b.WriteString(footerStyle.Render("模型响应中，输入暂不可用。"))
		b.WriteString("\n")
	}
	b.WriteString(contentStyle.Render(">>> " + m.input))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render("快捷键: Ctrl+C 退出，Esc 取消当前流式"))
	b.WriteString("\n")
	b.WriteString(footerStyle.Render("命令: /clear 清空会话"))
	if m.notice != "" {
		b.WriteString("\n")
		b.WriteString(noticeStyle.Render(m.notice))
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
