# ComputeCommander UI Layout

```
┌─────────┬─────────────────────────────────────────────┬──────────┐
│         │                                             │          │
│         │           Agent Session                     │          │
│   FP    │           (i.e. Claude Code)                │  Agents  │
│         │                                             │          │
│         │                                             │          │
│         │                                             │          │
│         │                                             │          │
│         │                                             │          │
│         │                                             │          │
│         ├───────────┬─────────┬─────────────┬─────────┤          │
│         │  Event    │  Mail   │   Merge     │ Events  │          │
│         │  Log      │         │   Queue     │         │          │
└─────────┴───────────┴─────────┴─────────────┴─────────┴──────────┘
```

## Panels

| Panel         | Position      | Description                        |
|---------------|---------------|------------------------------------|
| FP            | Left sidebar  | File picker / navigation           |
| Agent Session | Center main   | Primary workspace (Claude Code, etc.) |
| Agents        | Right sidebar | Agent list / management            |
| Event Log     | Bottom bar    | System events and logs             |
| Mail          | Bottom bar    | Messages / notifications           |
| Merge Queue   | Bottom bar    | Pending merges / PRs               |
| Events        | Bottom bar    | Activity feed                      |
