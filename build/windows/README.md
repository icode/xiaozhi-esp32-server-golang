# Windows 运行目录说明

本目录用于在 Windows 下直接运行小智服务（`start.bat` 启动）。以下为端口与访问方式说明。

## 端口与服务

| 端口 | 配置来源 | 说明 |
|------|----------|------|
| **8080** | `manager.json` → `server.port` | **内控管理服务**：HTTP API + 管理后台前端（发版嵌入时） |
| **8989** | `main_config.yaml` → `websocket.port` | **主服务 WebSocket**：设备/客户端连接用 |
| **9000** | `asr_server.json` → `server.port` | **内嵌 ASR/声纹服务**：语音识别与声纹相关接口 |
| **2883** | `main_config.yaml` → `mqtt_server.listen_port` | **MQTT 服务**：设备 MQTT 连接 |
| **8990** | `main_config.yaml` → `udp.listen_port` | **UDP 服务**：设备 UDP 通信 |
| **6060** | `main_config.yaml` → `server.pprof.port` | **pprof**：性能分析（默认关闭，需在配置中开启） |

## 如何访问

### 管理后台（内控）

- **地址**：`http://localhost:8080/`（本机）或 `http://<本机IP>:8080/`（局域网其他设备）
- **说明**：发版时若已用 `build_release.bat` 打包并带 `embed_ui`，直接打开上述地址即可进入管理后台登录页；API 与 WebSocket 为同源 `/api`、`/ws`。

### 设备/客户端连接主服务

- **WebSocket**：`ws://<服务器IP>:8989/...`（具体路径以主程序约定为准）
- **MQTT**：`<服务器IP>:2883`
- **UDP**：`<服务器IP>:8990`

### 内嵌 ASR/声纹服务

- 由主程序内部使用，默认访问 `http://127.0.0.1:9000`，一般无需在浏览器直接访问。

## 修改端口

- 管理后台端口：编辑 `manager.json` 中 `server.port`（如改为 `8080` 以外的端口）。
- 主服务 WebSocket/MQTT/UDP：编辑 `main_config.yaml` 中 `websocket.port`、`mqtt_server.listen_port`、`udp.listen_port` 等。
- 修改后需重启 `xiaozhi_server.exe` 生效。
