# 別PCのCodexへ渡す引き継ぎプロンプト

以下を、新しいPCでcloneしたリポジトリをワークスペースに設定したCodexへ、そのまま貼り付けてください。

```text
プロジェクト名は「在庫管理ツール（Zaiko Kanri）」です。
GitHubは https://github.com/qurioucitywork-dev/zaiko-kanri です。
作業対象ブランチは codex/react-gorm-aws-foundation です。
ローカルフォルダー名は zaiko-kanri に統一してください。

このプロジェクトは、基準画面の見た目を維持しながら、React + JavaScript、Go REST API、GORM、PostgreSQLへ統合している在庫管理システムです。AWS接続は後から切り替えられる設計を維持し、現在は課金が発生しないローカルPostgreSQL・ローカルファイル保存で作業してください。AWSへ実デプロイしないでください。

最初に、変更を加えず次を確認してください。
1. pwdとワークスペースがcloneしたzaiko-kanriであること
2. git remote -v が qurioucitywork-dev/zaiko-kanri を指すこと
3. git branch --show-current が codex/react-gorm-aws-foundation であること
4. git status -sb に未保存の変更がないこと
5. README.md、docs/other-pc-handoff.md、docs/target-architecture.md、docs/data-storage-ownership.mdを読むこと
6. scripts/bootstrap-other-pc.ps1を実行し、依存関係とテストを確認すること

起動は次を使用してください。
.\scripts\dev-docker-postgres.ps1 -Port 18086
管理画面は http://127.0.0.1:18086/app/app.html です。

開発上の必須ルール:
- 8080の旧コピーやGenspark側ではなく、このclone内の18086用統合環境を編集すること
- 見た目を崩さず、frontend/public/admin-reference とGo API/PostgreSQLの連動を優先すること
- frontend/public/admin-referenceの基準画面を変更した場合、internal/web/react-dist/admin-referenceの配信用コピーにも同じ変更を反映すること
- APP_DATAやlocalStorageだけを正本にせず、複数PCで共有すべき業務データはGo REST API経由でPostgreSQLへ保存すること
- migrationは既存番号を変更せず、新しい連番で追加すること
- 既存の管理番号、固定コード、伝票番号を表示順から再採番しないこと
- 仕入伝票の確定明細と在庫商品の生成元を一対一で保持し、全ステータス在庫件数と確定仕入明細の商品件数の不一致を再発させないこと
- 下書き仕入明細は在庫点数へ含めないこと
- PUBLICリポジトリなので、.env、DB dump、パスワード、実顧客データ、.data、.logs、.backupsをcommitしないこと
- ユーザーがpushを指示するまでは勝手にpushしないこと
- 既存の未コミット変更をreset、checkout、削除しないこと

変更後の最低検証:
1. npm run test:reference（frontendディレクトリ）
2. go test ./internal/persistence ./internal/web
3. 必要に応じて go test ./...
4. git diff --check
5. 管理者・作業者双方の対象画面を18086で目視確認
6. APIの在庫全ページを取得し、仕入確定明細との件数・管理番号対応を確認

まず現在のGit状態、起動可否、DB migration適用状況、テスト結果を短く報告してください。その後は私の追加指示を待ち、現在のデザインとデータ連動を維持しながら編集を続けてください。
```
