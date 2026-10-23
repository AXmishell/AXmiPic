# 目录结构

```
cmd/axmipic/          程序入口
internal/
  api/                HTTP 路由与处理器
  auth/               主体身份、密码、JWT、令牌、限流
  config/             配置加载与校验
  imaging/            图片处理（纯 Go / libvips）
  plugin/             运行时插件框架（WASM 沙箱、宿主能力、注册表）
  secret/             AES-256-GCM 加密
  server/             HTTP 服务器生命周期
  service/            业务逻辑（账户、上传、处理、存储、管理）
  storage/            存储后端与管理器（本地 / S3 / 七牛）
  store/              数据持久化（GORM）
  webui/              内嵌前端资源
configs/              配置示例
web/                  前端源码（Vue 3）
```
