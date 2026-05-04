## 1. 重构图片处理管线

- [x] 1.1 将 `downloadAndResizeImage` 拆分为独立函数：`downloadImage`（下载解码）、`resizeImage`（缩放）、`saveImage`（编码保存）
- [x] 1.2 在 `fetchFeed` 中调整调用逻辑：有图片走"下载+缩放+叠加"路径，下载失败或无图片走"色卡+叠加"路径，确保 `imagePath` 不再为空

## 2. 实现色卡生成

- [x] 2.1 定义深色调色盘（10 种颜色，RGB 各分量 ≤ 120）
- [x] 2.2 实现 `generateColorCard(title string, w, h int) *image.RGBA`：对 title 做 FNV-32a 哈希，mod 调色盘长度选色，填充整张图片

## 3. 实现文字叠加

- [x] 3.1 实现 `wrapText(text string, maxWidth, charWidth int) []string`：按字符边界折行，返回最多 3 行；第 3 行超出时截断末尾并追加 `…`
- [x] 3.2 实现 `overlayText(img *image.RGBA, title string, pubTime time.Time)`：调用 `wrapText` 计算行数，动态计算横条高度 = (行数+1)×14 + 6px，在图片底部绘制 alpha=160 半透明黑色横条
- [x] 3.3 使用 `golang.org/x/image/font/basicfont` 的 `Face7x13` 逐行绘制白色标题文字（左边距 6px，行间距 14px，从横条顶部内边距起算）
- [x] 3.4 在横条最底行绘制白色时间字符串，格式 `2006-01-02 15:04` UTC

## 4. 集成与验证

- [x] 4.1 更新 `backfillImages`：仅对远程 URL 条目重新下载+缩放，不重新叠加文字（保持原有逻辑）
- [x] 4.2 运行 `go vet ./...` 确认无静态错误
- [ ] 4.3 本地启动服务，检查新条目图片是否包含标题和时间叠加，无图片条目是否生成色卡
