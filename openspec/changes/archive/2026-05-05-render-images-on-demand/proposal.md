## Why

当前后端会在 RSS 轮询时预下载、缩放并落盘保存图片，即使这些条目从未被设备请求也会消耗网络、CPU 和磁盘。将图片处理改为按需渲染可以简化采集流程，并把失败降级统一收敛到图片访问时处理。

## What Changes

- 将条目图片处理从“抓取时预处理并保存本地文件”改为“访问 `/image/{id}.jpg` 时现场下载、渲染并返回 JPEG”
- 将 `items` 表中的图片字段从本地文件路径调整为原始图片 URL，用于按需下载上游图片
- 对所有条目统一暴露本地图片接口；当条目无原始图片、下载超时或下载失败时，图片接口返回纯色色卡并叠加标题与发布时间
- 新增图片下载超时配置项，默认由配置文件控制，本次目标值为 3 秒
- 更新图片相关日志与 Prometheus 指标，覆盖渲染请求、下载失败和色卡回退
- **BREAKING** 删除本地图片落盘与历史图片回填逻辑，旧数据不做兼容迁移

## Capabilities

### New Capabilities
- `on-demand-item-images`: 后端为每个 RSS 条目提供统一的按需渲染图片接口，并在失败时回退为纯色色卡

### Modified Capabilities

无

## Impact

- `server/rss/worker.go`：轮询阶段只提取并保存原始图片 URL，不再下载、缩放、叠字或写磁盘
- `server/image/handler.go`：从静态文件服务改为按需下载、缩放、叠字、编码并返回 JPEG
- `server/api/handlers.go`：`/v1/device/{device_id}/next` 对每个条目都返回本地 `/image/{id}.jpg` 地址
- `server/models/models.go`、`server/config/config.go`、`server/metrics/metrics.go`：分别调整数据模型、配置项和观测指标
- `config.yaml`：新增图片下载超时配置
