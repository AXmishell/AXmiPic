import type { ImageItem } from '@/api/types'

/** 未知尺寸时按 4:3 估算高度权重。 */
export function masonryAspectRatio(item: ImageItem): number {
  if (item.width > 0 && item.height > 0) return item.height / item.width
  return 0.75
}

/**
 * 把图片按“最短列”分配到若干列，使各列高度更均衡，且顺序比 CSS columns 更自然。
 * 返回每个列内的图片数组（列数固定为 columnCount）。
 */
export function buildMasonryColumns(items: ImageItem[], columnCount: number): ImageItem[][] {
  const n = Math.max(1, columnCount)
  const columns: ImageItem[][] = Array.from({ length: n }, () => [])
  const heights = new Array<number>(n).fill(0)
  for (const item of items) {
    let min = 0
    for (let i = 1; i < n; i += 1) {
      if (heights[i] < heights[min]) min = i
    }
    columns[min].push(item)
    heights[min] += masonryAspectRatio(item)
  }
  return columns
}

/** 根据容器宽度给出瀑布流列数。 */
export function masonryColumnCount(width: number): number {
  if (width < 560) return 2
  if (width < 880) return 3
  if (width < 1200) return 4
  return 5
}
