## Context

当前 `server/rss/worker.go` 中的 `downloadAndResizeImage` 函数负责下载远程图片、用最近邻缩放至 320×240，然后保存为 JPEG。条目无图片时 `image_path` 为空，设备端收到空路径后需自行处理。

项目约束：CGO 禁用（`CGO_ENABLED=0`）、纯 Go 标准库 + 已有 vendor 依赖、无测试、无外部图片处理服务。已有依赖 `golang.org/x/image`（含 `draw`、`font` 子包）和 `golang.org/x/image/math/fixed`，可直接复用。

## Goals / Non-Goals

**Goals:**
- 每个保存的条目都产出一张本地 JPEG，包含标题和发布时间叠加文字
- 有图片：在缩放后的图片底部叠加半透明黑色背景条 + 白色文字（标题 + 时间）
- 无图片：按标题哈希从固定调色盘中选色，生成 320×240 纯色背景图，再叠加相同样式文字
- 不引入新的外部依赖（使用内嵌位图字体 `golang.org/x/image/font/basicfont`，已在 vendor 中）

**Non-Goals:**
- 不对历史条目重新叠加文字（backfill 仅补下载，不重新处理已有本地图片）
- 不支持动态字体大小或多行自动换行（固定单行截断）
- 不变更 API 或数据库 schema

## Decisions

### 1. 字体选择：`basicfont.Face7x13`

**决策**：使用 `golang.org/x/image/font/basicfont` 提供的 7×13 像素位图字体。

**理由**：已在 vendor 中，零新依赖，无需 embed 外部字体文件，CGO 兼容，渲染逻辑简单。

**替代方案**：TrueType 字体（需 `github.com/golang/freetype` 或 `golang.org/x/image/font/opentype`）——字体更美观，但需引入新依赖或 embed 字体文件，超出本次变更范围。

### 2. 色卡生成：标题哈希 mod 调色盘

**决策**：取条目 `title` 字符串的 FNV-32a 哈希值，mod 一个预定义的 10 色调色盘（深色系，确保白字可读），填充整张 320×240 图片。

**理由**：相同标题始终得到相同颜色（幂等），实现简单，无随机性，不依赖外部资源。

**替代方案**：渐变色/纹理背景——视觉更丰富，但实现复杂度高，不值得在此阶段引入。

### 3. 文字叠加区域：底部动态高度横条

**决策**：根据标题实际行数动态计算横条高度。排版策略按优先级依次应用：
1. 标题单行可容纳 → 单行渲染
2. 单行超出 → 按字符边界折行，最多 3 行
3. 超过 3 行 → 保留前 3 行，最后一行截断加 `…`

横条高度 = (标题行数 + 1) × 14px + 6px 顶部内边距，底边与图片底边对齐，alpha=160。时间固定渲染在最底行。

**理由**：basicfont 字高 13px、字宽 7px，在 320px 宽度下单行最多容纳约 44 个 ASCII 字符；新闻标题普遍 20-80 字符，允许最多 3 行可覆盖绝大多数场景，同时将横条高度控制在图片高度的约 25% 以内（最大 ~60px/240px）。

**替代方案**：固定单行截断——实现更简单，但短标题浪费空间、长标题信息丢失过多，不如动态排版友好。

### 4. 重构 `downloadAndResizeImage` → 拆分为独立步骤

**决策**：将现有函数拆为：
- `downloadImage(url) image.Image` — 仅下载解码
- `resizeImage(src, w, h) *image.RGBA` — 缩放
- `overlayText(img, title, pubTime)` — 叠加文字
- `generateColorCard(title, w, h) *image.RGBA` — 生成色卡
- `saveImage(img, dir, date) string` — 编码保存

在 `fetchFeed` 中按"有图 → 下载+缩放+叠加 / 无图 → 色卡+叠加"两条路径调用。

**理由**：职责清晰，便于独立测试（未来），避免单一函数膨胀。

## Risks / Trade-offs

- [风险] 标题含中文/非 ASCII 字符时 basicfont 仅能渲染 ASCII，非 ASCII 字符显示为方块 → **缓解**：在 proposal 范围内接受此限制；后续可替换字体。
- [风险] 网络超时或图片解码失败时原先返回空路径，新逻辑应降级为色卡而非失败 → **缓解**：在 `fetchFeed` 中捕获下载失败，统一走色卡路径。
- [Trade-off] 每条新条目都保存图片（即使无原始图），磁盘用量略增 → 可接受，图片体积固定（约 15-30KB/张）。
