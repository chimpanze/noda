# Noda Visual Editor

The visual workflow editor for Noda. A React application that reads and writes Noda's JSON config files, providing a graphical interface for building workflows, managing routes, configuring services, and debugging executions in real time.

## Tech Stack

- React + TypeScript (Vite)
- [React Flow](https://reactflow.dev/) (@xyflow/react) for the workflow canvas
- [shadcn/ui](https://ui.shadcn.com/) for UI components
- [Zustand](https://github.com/pmndrs/zustand) for state management
- [ELKjs](https://github.com/kieler/elkjs) for auto-layout
- [Monaco Editor](https://microsoft.github.io/monaco-editor/) for JSON/expression editing

## Development

```bash
cd editor

# Install dependencies
npm install

# Start dev server
npm run dev

# Run tests
npm test

# Lint
npm run lint

# Build for production
npm run build
```

The editor is embedded into the Go binary via `editorfs/` for production use. During development, run it standalone and it connects to the Noda dev server.

## Architecture

See [docs/_internal/visual-editor.md](../docs/_internal/visual-editor.md) for the full editor design document.
