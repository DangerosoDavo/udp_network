# UDP Network Protocol - Usage Guide

## Running Server and Client

### Terminal 1 - Server
```bash
./bin/server
```

You should see:
```
2025/05/24 02:32:55 Server starting...
2025/05/24 02:32:55 UDP server running on :8080
```

When a client connects, you'll see:
```
2025/05/24 02:32:55 Client connected: 127.0.0.1:38150
```

When the client sends a message, the SERVER terminal will show:
```
2025/05/24 02:32:56 [127.0.0.1:38150] Text: Hello from client
```

### Terminal 2 - Client
```bash
./bin/client localhost:8080
```

You should see:
```
UDP Network Client
Commands:
  /stats         - Get server statistics
  /clients       - List connected clients
  /broadcast <msg> - Broadcast message to all clients
  /ping          - Ping the server
  /stress <n>    - Send n messages rapidly
  /quit          - Exit client

Type messages to send to server:
> 
```

Type a message like "Hello from client" and press Enter.

The CLIENT terminal will show the server's JSON response:
```
[Server JSON]:
{
  "client": "127.0.0.1:38150",
  "echo": "Hello from client",
  "length": 17,
  "serverTime": "2025-05-24T02:32:56+01:00"
}
```

## Important Notes

1. **Server logs are in the SERVER terminal** - When you send a message from the client, look at the server terminal to see the log output.

2. **Client shows JSON responses** - The client terminal shows the server's JSON responses, not plain text logs.

3. **Both terminals are needed** - Keep both terminals open to see the full communication flow.

## Example Session

**Server Terminal:**
```
2025/05/24 02:32:55 Server starting...
2025/05/24 02:32:55 UDP server running on :8080
2025/05/24 02:32:55 Client connected: 127.0.0.1:38150
2025/05/24 02:32:56 [127.0.0.1:38150] Text: Hello
2025/05/24 02:32:58 [127.0.0.1:38150] Text: How are you?
```

**Client Terminal:**
```
> Hello
[Server JSON]:
{
  "client": "127.0.0.1:38150",
  "echo": "Hello",
  "length": 5,
  "serverTime": "2025-05-24T02:32:56+01:00"
}
> How are you?
[Server JSON]:
{
  "client": "127.0.0.1:38150",
  "echo": "How are you?",
  "length": 12,
  "serverTime": "2025-05-24T02:32:58+01:00"
}
```

## Testing Commands

Try these commands in the client:
- `/stats` - Get server statistics
- `/ping` - Ping the server (response: "pong")
- `/clients` - List all connected clients
- `/broadcast Hello everyone` - Broadcast to all clients