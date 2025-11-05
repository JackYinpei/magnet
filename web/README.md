## Magnet Player 前端

该目录包含基于 Next.js 14 App Router 构建的控制台，提供以下能力：

- 创建新的磁力下载任务；
- 查看任务状态、下载进度与上传结果；
- 浏览对象存储（S3/R2 兼容）中已上传的内容，可按前缀过滤。

前端所有接口请求都会通过环境变量 `NEXT_PUBLIC_API_BASE_URL` 拼接，例如：

```
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

> 如果未设置该变量，默认会使用 `http://localhost:8080`。

## 本地开发

在 `web` 目录中安装依赖并启动开发服务器：

```bash
cd web
npm install
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080 npm run dev
```

浏览器访问 [http://localhost:3000](http://localhost:3000) 即可。

## 构建

要产出可部署的静态资源：

```bash
cd web
npm run build
npm run start
```

确保后端服务和 `.env` 中的 S3/R2 配置与前端使用的 API 地址保持一致。
