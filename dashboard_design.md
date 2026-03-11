
## Communication Protocol: JSON-RPC over WebSockets
To ensure low-latency, real-time updates for the HUD dashboard, we will use JSON-RPC 2.0 over WebSockets. This allows:
- **Server-to-Client (Push):** Agents push status updates (started, progress, completed) directly to the dashboard.
- **Client-to-Server (Command):** Dashboard can send control signals (pause, stop, re-prioritize) to agents.

### Message Schema
```json
{
  "jsonrpc": "2.0",
  "method": "agent.status_update",
  "params": {
    "agent_id": "string",
    "status": "string",
    "task": "string",
    "timestamp": "string"
  }
}
```
