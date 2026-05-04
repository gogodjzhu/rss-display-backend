## Why

当前图片处理管线仅下载并缩放图片，ESP32 设备收到图片后还需在端侧渲染标题和时间，增加了设备复杂度。将文字叠加移到后端，可以让设备直接显示成品图片，同时解决"无图片"条目完全没有视觉内容的问题。

## What Changes

- 在图片下载/缩放后，将条目标题和发布时间叠加渲染到图片上（白色文字 + 半透明黑色背景条）
- 当条目没有任何图片时，生成一张基于条目标题哈希的纯色色卡作为背景，再叠加标题和时间
- 去掉原有"无图片则 image_path 为空"的逻辑——每个条目都保证有本地图片

## Capabilities

### New Capabilities

- `image-text-overlay`: 在已缩放到 320×240 的图像上叠加标题文字与发布时间（半透明黑底白字，位于图片底部）
- `color-card-generator`: 当 RSS 条目无图片时，按标题哈希生成固定调色盘中的纯色背景图（320×240）

### Modified Capabilities

（无现有规格层行为变更）

## Impact

- `server/rss/worker.go`：`downloadAndResizeImage` 重构，新增 `overlayText` 和 `generateColorCard` 函数
- 无 API 变更，无数据库 schema 变更
- 新增依赖：嵌入式字体文件（Go embed），以及 `golang.org/x/image/font` 或轻量级位图字体库用于文字渲染
- 历史条目的 image_path 不受影响（backfill 逻辑不重新叠加文字）
