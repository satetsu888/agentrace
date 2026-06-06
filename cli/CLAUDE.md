# CLI 開発ガイド

Claude Code の transcript を AgenTrace サーバーに送信する CLI ツール。

## 技術スタック

- Node.js / TypeScript
- Commander.js（CLI フレームワーク）
- tsup（ビルド、esbuild ベース）
- npx 配布

## ディレクトリ構成

```
cli/src/
├── index.ts             # エントリーポイント（Commander.js）
├── globals.d.ts         # グローバル定数の型定義（__CLI_VERSION__）
├── commands/            # コマンド実装
│   ├── init.ts          # 初期セットアップ（ブラウザ連携）
│   ├── login.ts         # Webログイン
│   ├── send.ts          # transcript送信（hooks用 + 手動送信）
│   ├── mcp-server.ts    # MCPサーバー
│   ├── doctor.ts        # 設定状況・接続確認
│   ├── on.ts            # hooks有効化
│   ├── off.ts           # hooks無効化
│   └── uninstall.ts     # 完全アンインストール
├── config/              # 設定管理
│   ├── manager.ts       # ~/.agentrace/config.json CRUD
│   └── cursor.ts        # 差分追跡（送信済み行数）
├── hooks/               # Claude Code hooks連携
│   └── installer.ts     # ~/.claude/settings.json 編集
├── mcp/                 # MCP関連
│   └── plan-document-client.ts # PlanDocument APIクライアント
└── utils/               # ユーティリティ
    ├── http.ts          # HTTP APIクライアント
    ├── proxy.ts         # プロキシ設定
    ├── callback-server.ts # ローカルHTTP callbackサーバー
    ├── browser.ts       # ブラウザ起動
    └── session-finder.ts # Claude Codeセッションファイル検索
```

## 設計方針

### 責務分離

| レイヤー | 責務 |
|---------|------|
| commands/ | ユーザーコマンドの処理フロー |
| config/ | データ永続化（設定、カーソル位置） |
| hooks/ | Claude Code連携（settings.json編集） |
| utils/ | 外部サービス連携（HTTP、ブラウザ） |

### エラーハンドリング

- **send コマンド（hooks経由）**: すべてのエラーで `exit(0)` → hooks をブロックしない
- **send コマンド（手動）**: エラーで `exit(1)` → ユーザーにエラーを通知
- **init コマンド**: 致命的エラーで `exit(1)` → ユーザーに再試行を促す

### 差分送信の仕組み

1. `~/.agentrace/cursors/{session_id}.json` で送信済み行数を管理
2. JSONL を読み込み、カーソル位置以降の行のみ抽出
3. 送信成功後にカーソル位置を更新

### Git 情報の取得

- 初回送信時のみ取得（パフォーマンス）
- `CLAUDE_PROJECT_DIR` 環境変数を優先
- 未設定時は stdin の `cwd` にフォールバック

## コマンド一覧

| コマンド | 説明 |
|---------|------|
| `init --url <url>` | 初期設定 + hooks + MCP インストール |
| `init --url <url> --proxy <proxy-url>` | プロキシ経由で接続 |
| `init --url <url> --dev` | 開発モード（ローカルCLIパス使用） |
| `init --url <url> --async` | 非同期送信モードを有効化（`send_mode: async`） |
| `init --url <url> --local` | プロジェクト単位で hooks/MCP を設定 |
| `init --url <url> --local --separate-local-config` | プロジェクト単位で config も作成 |
| `login` | Webログイン URL 発行 |
| `send` | transcript 差分送信（hooks用、stdin から JSON 受け取り） |
| `send --claude-session-id <id>` | 既存セッションを手動送信（差分のみ） |
| `mcp-server` | MCPサーバー起動（stdio通信） |
| `on` / `off` | hooks + MCP 有効化/無効化 |
| `on --async` | `send_mode` を async に切替（保存） |
| `on --local` / `off --local` | プロジェクト単位で hooks + MCP 有効化/無効化 |
| `uninstall` | hooks/MCP/config 削除 |
| `uninstall --local` | プロジェクト単位の hooks/MCP/config 削除 |
| `doctor` | 設定状況・サーバー接続確認 |

## 設定ファイル

### 設定の読み込み優先順位

設定は以下の優先順位で読み込まれる:

1. **ローカル設定**: カレントディレクトリから親ディレクトリを遡り、最初に見つかった `{dir}/.agentrace/config.json`
2. **グローバル設定**: `~/.agentrace/config.json`

これにより、プロジェクトのサブディレクトリにいても、プロジェクトルートのローカル設定が使用される。

`doctor` コマンドで現在どの設定が使用されているか確認できる。

### グローバル設定

#### ~/.agentrace/config.json

```json
{
  "server_url": "http://localhost:8080",
  "api_key": "agtr_xxxxxxxxxxxxxxxxxxxxxxxx",
  "proxy_url": "http://proxy.example.com:8080",
  "send_mode": "async"
}
```

**proxy_url** はオプション。設定しない場合は環境変数 `HTTPS_PROXY` / `HTTP_PROXY` にフォールバックする。

**send_mode** はオプション（`"sync"` | `"async"`、未設定は `"sync"`）。

| モード | 挙動 |
|--------|------|
| `sync`（既定） | hook が送信（HTTPS 往復）の完了を待つ。従来どおり。 |
| `async` | hook は detached worker を spawn して即 return し、送信は背後で行う。worker は per-session ロックで同一セッションの送信を直列化する。 |

`async` への切替は `init --async` / `on --async`、確認は `doctor` の `Send mode` 行で行う。手動送信（`--claude-session-id`）は常に同期。

### ローカル設定（--local オプション使用時）

`--local` オプションを使うと、プロジェクト単位で AgenTrace を有効/無効にできる。

#### プロジェクトローカルの hooks

`{project}/.claude/settings.json` に hooks が追加される（グローバルの `~/.claude/settings.json` ではなく）。

#### プロジェクトローカルの MCP

`~/.claude.json` の `projects.{project_path}.mcpServers` に追加される（local スコープ）。

```json
{
  "projects": {
    "/path/to/project": {
      "mcpServers": {
        "agentrace": {
          "command": "npx",
          "args": ["agentrace", "mcp-server"]
        }
      }
    }
  }
}
```

#### プロジェクトローカルの config（--separate-local-config 使用時）

`{project}/.agentrace/config.json` に config が保存される。

**注意**: `.agentrace/` を `.gitignore` に追加すること（API キーを含むため）。

### ~/.agentrace/cursors/{session_id}.json

```json
{
  "lineCount": 123,
  "lastUpdated": "2024-01-01T00:00:00.000Z"
}
```

## Claude Code 設定

### ~/.claude/settings.json（hooks設定）

`init` または `on` コマンドで自動追加:

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "npx agentrace send"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "npx agentrace send"
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "npx agentrace send"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "npx agentrace send"
          }
        ]
      }
    ]
  }
}
```

### ~/.claude.json（MCP設定）

MCPサーバーは `settings.json` ではなく `~/.claude.json` に設定:

```json
{
  "mcpServers": {
    "agentrace": {
      "command": "npx",
      "args": ["agentrace", "mcp-server"]
    }
  }
}
```

## MCPサーバー

Claude Code の MCP (Model Context Protocol) サーバーとして動作し、PlanDocument の操作を提供。

### 提供するツール

| ツール | 説明 | 引数 |
|--------|------|------|
| `search_plans` | Planの検索（フィルタリング対応） | `git_remote_url`, `status`, `description` |
| `read_plan` | Plan読み込み | `id` |
| `create_plan` | Plan作成（project は session から自動取得） | `description`, `body` |
| `update_plan` | Plan更新（パッチ自動生成） | `id`, `body` |
| `set_plan_status` | Planステータス変更 | `id`, `status` |

**注**: `create_plan` と `update_plan` の `session_id` は PreToolUse hook により自動注入されます。

### 使用例

```
# Planの検索
search_plans(git_remote_url: "https://github.com/user/repo.git", status: "planning")

# 新規Planを作成（session_id は自動注入）
create_plan(
  description: "実装計画",
  body: "# 実装ステップ\n\n1. ..."
)
```

### PreToolUse Hook

`init` コマンドで `~/.agentrace/hooks/inject-session-id.sh` がインストールされ、`~/.claude/settings.json` に登録されます。

このhookは `mcp__agentrace__create_plan` と `mcp__agentrace__update_plan` の呼び出し時に `session_id` を自動注入します。

## Hooks の仕組み

transcript送信は以下の4つのタイミングで発火:

1. **UserPromptSubmit**: ユーザーがメッセージを送信した直後（10秒待機後に送信）
2. **Stop**: Claude Codeが応答を完了した時
3. **SubagentStop**: Taskエージェント（explore, plan等）が完了した時
4. **PostToolUse**: ツール使用完了後（リアルタイム更新用）

どのイベントでも同じ処理:
1. `~/.claude/settings.json` の該当hookを実行
2. stdin に JSON を渡して `npx agentrace send` を実行
3. CLI が stdin から JSON を読み取り、差分をサーバーに送信

### stdin JSON 形式

```json
{
  "session_id": "uuid",
  "transcript_path": "/path/to/transcript.jsonl",
  "cwd": "/current/working/directory"
}
```

## 手動送信

hooks を使わずに既存のセッションを手動で送信できる。

```bash
npx agentrace send --claude-session-id <session-id>
```

### 動作

1. `~/.claude/projects/` 配下を再帰検索し、`{session-id}.jsonl` を探す
2. JSONL 内の `type: "user"` エントリから `cwd` を抽出
3. 差分のみ送信（カーソル管理は hooks と共通）

### 注意点

- 会話データ（user/assistant メッセージ）を含むセッションファイルのみ有効
- `file-history-snapshot` のみのファイルはサーバー側でイベントとして処理されない

## 開発モード

`--dev` オプションを付けると、hooks/MCPコマンドが変わる:

| モード | コマンド |
|--------|----------|
| 本番 | `npx agentrace send` |
| 開発 | `npx tsx /path/to/cli/src/index.ts send` |

## 開発時の起動

```bash
npm install
npx tsx src/index.ts init --url http://localhost:8080 --dev
```

## ビルド

tsup を使用してビルド:

```bash
npm run build
```

### バージョン埋め込み

`tsup.config.ts` で `package.json` のバージョンを `__CLI_VERSION__` としてビルド時に埋め込む。

```typescript
// tsup.config.ts
define: {
  __CLI_VERSION__: JSON.stringify(pkg.version),
}
```

コード内では以下のように使用:

```typescript
// tsx 開発時は undefined になるのでフォールバック
const version = typeof __CLI_VERSION__ !== "undefined" ? __CLI_VERSION__ : "dev";
```

型定義は `src/globals.d.ts` で宣言。
