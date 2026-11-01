package memory

import "testing"

func TestMergeMemory_PreservesMissingLevels(t *testing.T) {
	old := MemoryContent{GlobalMemory: "old global", WorkspaceMemory: "old workspace"}

	got := mergeMemory(old, "<global>new global</global>")
	if got.GlobalMemory != "new global" {
		t.Errorf("GlobalMemory = %q, want %q", got.GlobalMemory, "new global")
	}
	if got.WorkspaceMemory != "old workspace" {
		t.Errorf("WorkspaceMemory = %q, want it preserved", got.WorkspaceMemory)
	}
}

func TestMergeMemory_ClearsExplicitEmptyTag(t *testing.T) {
	old := MemoryContent{GlobalMemory: "old global", WorkspaceMemory: "old workspace"}

	got := mergeMemory(old, "<global></global>")
	if got.GlobalMemory != "" {
		t.Errorf("GlobalMemory = %q, want empty", got.GlobalMemory)
	}
	if got.WorkspaceMemory != "old workspace" {
		t.Errorf("WorkspaceMemory = %q, want it preserved", got.WorkspaceMemory)
	}
}

func TestMergeMemory_MalformedOutputKeepsOldMemory(t *testing.T) {
	old := MemoryContent{GlobalMemory: "old global", WorkspaceMemory: "old workspace"}

	got := mergeMemory(old, "the model ignored the XML format")
	if got != old {
		t.Errorf("mergeMemory() = %+v, want old memory %+v", got, old)
	}
}

func TestExtractXMLTag_ReportsPresence(t *testing.T) {
	if value, ok := extractXMLTag("<global>hello</global>", "global"); !ok || value != "hello" {
		t.Errorf("extractXMLTag() = %q, %v; want hello, true", value, ok)
	}
	if _, ok := extractXMLTag("no tags here", "global"); ok {
		t.Error("extractXMLTag() reported a missing tag as present")
	}
}
