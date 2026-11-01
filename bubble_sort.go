// Package main 演示冒泡排序算法的实现
package main

import "fmt"

// bubbleSort 对整数切片进行冒泡排序（升序排列）
//
// 算法思路：
//   - 重复遍历数组，比较相邻的两个元素
//   - 如果前一个元素比后一个大，则交换它们的位置
//   - 每一轮遍历会将当前未排序部分的最大值"冒泡"到末尾
//
// 时间复杂度：O(n²)，最坏和平均情况；O(n)，最好情况（已有序）
// 空间复杂度：O(1)，原地排序
func bubbleSort(arr []int) {
	// 获取数组长度
	n := len(arr)

	// 外层循环：控制排序的轮数，共需要 n-1 轮
	for i := 0; i < n-1; i++ {
		// swapped 标记本轮是否发生过交换
		// 用于优化：如果一轮中没有交换，说明数组已经有序
		swapped := false

		// 内层循环：逐个比较相邻元素
		// 每轮结束后，末尾 i 个元素已经排好，所以只需比较前 n-1-i 个
		for j := 0; j < n-1-i; j++ {
			// 如果前一个元素大于后一个元素，则交换
			if arr[j] > arr[j+1] {
				// Go 语言特色的多重赋值，无需临时变量即可交换
				arr[j], arr[j+1] = arr[j+1], arr[j]
				swapped = true // 标记发生了交换
			}
		}

		// 如果本轮没有发生任何交换，说明数组已经完全有序，提前结束排序
		if !swapped {
			break
		}
	}
}

// main 程序入口，测试冒泡排序
func main() {
	// 待排序的数组
	arr := []int{64, 34, 25, 12, 22, 11, 90}
	fmt.Println("排序前:", arr)

	// 调用冒泡排序
	bubbleSort(arr)
	fmt.Println("排序后:", arr)
}
