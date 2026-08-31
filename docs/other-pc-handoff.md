# 別PCへの引き継ぎ・起動手順

## プロジェクト名

- 表示名: **在庫管理ツール（Zaiko Kanri）**
- ローカルフォルダー名: **`zaiko-kanri`**
- GitHub: <https://github.com/qurioucitywork-dev/zaiko-kanri>
- 引き継ぎブランチ: **`codex/react-gorm-aws-foundation`**

同名の別フォルダーや古いコピーを作ると、どちらを編集しているか分からなくなります。別PCでは上記フォルダー名を一つだけ使い、Codexにもそのフォルダーをワークスペースとして指定してください。

## 重要: Gitで移るもの・移らないもの

GitHubへ移るものは、ソースコード、DB migration、初期データ、テスト、文書、サンプルCSVです。次は安全上Gitへ登録しません。

- PostgreSQLの現在の実データ
- 商品画像のローカル実体
- `.env`と認証情報
- `.logs`、`.data`、`.backups`

したがって、別PCでcloneすれば同じ機能・同じDB構造・プレビュー初期データは再現できますが、このPCで後から入力した実データまでは自動で移りません。実データを引き継ぐ場合は、PUBLICリポジトリへDB dumpをpushせず、暗号化したバックアップを別経路で渡してください。

## 必要なアプリ（Windows）

1. [Git for Windows](https://git-scm.com/download/win)
2. [Docker Desktop](https://www.docker.com/products/docker-desktop/)
3. [Go 1.26以降](https://go.dev/dl/)
4. [Node.js LTS](https://nodejs.org/)
5. [GitHub CLI](https://cli.github.com/)（pushやPR作成を行う場合）
6. Codex desktop（編集を引き継ぐ場合）

Windows TerminalまたはPowerShellで一括導入する場合:

```powershell
winget install --id Git.Git --exact
winget install --id Docker.DockerDesktop --exact
winget install --id GoLang.Go --exact
winget install --id OpenJS.NodeJS.LTS --exact
winget install --id GitHub.cli --exact
```

Docker Desktopの初回起動時は、WSL2の有効化やWindows再起動を求められる場合があります。

## cloneとブランチ接続

```powershell
cd $HOME\Documents
git clone --branch codex/react-gorm-aws-foundation --single-branch https://github.com/qurioucitywork-dev/zaiko-kanri.git
cd .\zaiko-kanri
git remote -v
git branch --show-current
```

次の結果になれば接続先は正しい状態です。

- remote: `https://github.com/qurioucitywork-dev/zaiko-kanri.git`
- branch: `codex/react-gorm-aws-foundation`

## 初回セットアップ

Docker Desktopを起動してから実行します。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\bootstrap-other-pc.ps1
```

必須アプリもwingetで導入したい場合:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\bootstrap-other-pc.ps1 -InstallMissing
```

## 起動

```powershell
.\scripts\dev-docker-postgres.ps1 -Port 18086
```

ブラウザ:

- 管理画面: <http://127.0.0.1:18086/app/app.html>
- React入口: <http://127.0.0.1:18086/app/>

プレビュー用アカウント:

- 管理者: `admin` / `preview-admin-2026`
- 作業者: `worker` / `preview-worker-2026`
- ゲスト: `G001` / `preview-guest-2026`

## Codexで編集を続ける接続方法

1. Codex desktopで新しいタスクを作成します。
2. ワークスペースとしてcloneした `...\Documents\zaiko-kanri` を選びます。
3. [`CODEX_HANDOFF_PROMPT.md`](./CODEX_HANDOFF_PROMPT.md)のコードブロック全体を最初のメッセージへ貼り付けます。
4. 最初に `git status -sb`、remote、ブランチ、テスト結果を確認させます。
5. 作業前に必ず最新化します。

```powershell
git fetch origin
git pull --ff-only origin codex/react-gorm-aws-foundation
```

同じブランチを2台で同時編集すると競合しやすいため、旧PC側の作業を止めてpushした後、新PCでpullしてください。

## 実データを後から移す場合

現在のDBを正確に移す必要がある場合は、元PCでPostgreSQLのカスタム形式バックアップを作り、暗号化して別経路で転送します。PUBLIC GitHubへは絶対に置かないでください。

元PC（Docker PostgreSQLの場合）の例:

```powershell
New-Item -ItemType Directory -Force .\.backups | Out-Null
docker compose exec -T postgres pg_dump -U zaiko -d zaiko -Fc -f /tmp/zaiko-handoff.dump
docker compose cp postgres:/tmp/zaiko-handoff.dump .\.backups\zaiko-handoff.dump
```

新PCへ安全に転送した後の復元は既存DBを置き換えるため、必ずバックアップを確認してから行います。

```powershell
docker compose up -d postgres
docker compose cp .\.backups\zaiko-handoff.dump postgres:/tmp/zaiko-handoff.dump
docker compose exec -T postgres pg_restore --clean --if-exists --no-owner -U zaiko -d zaiko /tmp/zaiko-handoff.dump
```

商品画像も必要な場合は、元PCの `.data\uploads` を暗号化して別経路で新PCの同じ場所へ移します。

## 更新をGitHubへ戻す手順

```powershell
git status -sb
git diff --check
git add <変更したファイル>
git commit -m "変更内容を短く記載"
git push origin codex/react-gorm-aws-foundation
```

`.env`、`.data`、`.logs`、`.backups`、実際の顧客情報・パスワード・DB dumpはcommitしません。
