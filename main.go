package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "os/exec"
    "strings"

    "github.com/gorilla/websocket"
    "golang.org/x/net/webdav"
)

var (
    username = "admin" // 用户名
    password = "admin" // 密码
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true // 允许所有来源的 WebSocket 连接
    },
}

// BasicAuth 中间件，用于验证用户名和密码
func basicAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth() // 从请求头中获取 Basic Auth 信息
        if !ok || user != username || pass != password {
            w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`) // 要求客户端提供认证信息
            http.Error(w, "未经授权", http.StatusUnauthorized) // 返回 401 错误
            return
        }
        next(w, r) // 认证通过，继续处理请求
    }
}

// 处理 WebSocket 连接
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil) // 升级 HTTP 连接为 WebSocket 连接
    if err != nil {
        log.Println("升级连接失败:", err)
        return
    }
    defer conn.Close() // 确保连接关闭

    // 初始化当前工作目录
    currentDir, _ := os.Getwd()

    for {
        _, message, err := conn.ReadMessage() // 读取客户端发送的消息
        if err != nil {
            log.Println("读取消息失败:", err)
            break
        }

        cmd := string(message) // 将消息转换为字符串
        if strings.HasPrefix(cmd, "cd ") {
            // 处理 cd 命令
            newDir := strings.TrimSpace(cmd[3:]) // 获取目标目录
            err := os.Chdir(newDir)              // 切换目录
            if err != nil {
                output := []byte(fmt.Sprintf("错误: %s\n", err)) // 返回错误信息
                conn.WriteMessage(websocket.TextMessage, output)
            } else {
                currentDir, _ = os.Getwd() // 更新当前目录
                output := []byte(fmt.Sprintf("已切换到目录: %s\n", currentDir))
                conn.WriteMessage(websocket.TextMessage, output)
            }
        } else {
            // 执行其他命令
            cmdExec := exec.Command("sh", "-c", cmd) // 在 shell 中执行命令
            cmdExec.Dir = currentDir                // 设置命令执行的工作目录
            output, err := cmdExec.CombinedOutput() // 获取命令输出
            if err != nil {
                output = []byte(fmt.Sprintf("错误: %s\n", err)) // 返回错误信息
            }
            conn.WriteMessage(websocket.TextMessage, output) // 将输出发送给客户端
        }
    }
}

// 处理 WebShell 页面请求
func serveWebShell(w http.ResponseWriter, r *http.Request) {
    if r.Method != "GET" {
        http.Error(w, "方法不允许", http.StatusMethodNotAllowed) // 返回 405 错误
        return
    }

    w.Header().Set("Content-Type", "text/html") // 设置响应头
    w.Write([]byte(`
        <!DOCTYPE html>
        <html lang="zh">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <title>网页终端</title>
            <style>
                #output {
                    width: 100%;
                    height: 300px;
                    background-color: black;
                    color: white;
                    padding: 10px;
                    overflow-y: scroll;
                    font-family: monospace;
                    white-space: pre-wrap;
                }
                #input {
                    width: 100%;
                    padding: 10px;
                    font-family: monospace;
                }
            </style>
        </head>
        <body>
            <div id="output"></div>
            <input type="text" id="input" placeholder="输入命令...">

            <script>
                const output = document.getElementById('output');
                const input = document.getElementById('input');

                // 根据当前页面的协议自动选择 ws 或 wss
                const protocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
                const ws = new WebSocket(protocol + window.location.host + '/ws');

                // 接收服务器返回的消息并显示
                ws.onmessage = function(event) {
                    const line = document.createElement('div');
                    line.textContent = event.data;
                    output.appendChild(line);
                    output.scrollTop = output.scrollHeight;
                };

                // 监听输入框的回车事件
                input.addEventListener('keyup', function(event) {
                    if (event.key === 'Enter') {
                        const command = input.value;
                        ws.send(command); // 发送命令到服务器
                        input.value = ''; // 清空输入框
                    }
                });
            </script>
        </body>
        </html>
    `))
}

func main() {
    // 从环境变量中获取端口，如果未设置则使用默认端口 8080
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    // 创建 WebDAV 文件系统处理器，根目录设置为 Linux 根目录 "/"
    webdavHandler := &webdav.Handler{
        FileSystem: webdav.Dir("/"), // 设置为根目录
        LockSystem: webdav.NewMemLS(),
    }

    // 根路径处理函数，根据 Method 区分 WebDAV 和 WebShell
    http.HandleFunc("/", basicAuth(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == "GET" && r.URL.Path == "/" {
            // 如果是 GET 请求且路径是根路径，返回 WebShell 页面
            serveWebShell(w, r)
        } else {
            // 否则交给 WebDAV 处理
            webdavHandler.ServeHTTP(w, r)
        }
    }))

    // WebSocket 路由
    http.HandleFunc("/ws", basicAuth(handleWebSocket))

    log.Printf("服务器已启动，监听端口 :%s\n", port)
    log.Fatal(http.ListenAndServe(":"+port, nil)) // 启动 HTTP 服务器
}
