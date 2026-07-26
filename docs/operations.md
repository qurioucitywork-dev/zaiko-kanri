# 運用手順

## リリース前確認

PowerShellの実行ポリシーでローカルスクリプトが無効な場合も含め、次のコマンドでテスト、静的解析、本番用バイナリ2本を生成します。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-check.ps1
```

生成物：

- `bin\zaiko-kanri.exe`
- `bin\zaiko-maintenance.exe`

## 初回セットアップ

1. `.env.production.example`を参考にサービス実行環境へ環境変数を設定します。
2. DBとアップロード先にはサービスアカウントだけが書き込める権限を設定します。
3. 12文字以上の初期パスワードをUTF-8ファイルへ一時保存します。
4. 初期組織と管理者を作成します。

```powershell
.\bin\zaiko-maintenance.exe bootstrap-admin `
  -database C:\ProgramData\ZaikoKanri\data\zaiko.db `
  -organization-code WATCH `
  -organization-name "ウォッチ株式会社" `
  -username admin `
  -display-name "初期管理者" `
  -password-file C:\SecureTemp\initial-password.txt
```

作成後はパスワードファイルを安全に削除し、`ZAIKO_ORGANIZATION_CODE`を同じ組織コードへ設定します。コマンドライン引数へパスワード自体を指定する方式は、プロセス一覧や履歴への露出を防ぐため用意していません。

## 本番起動条件

- `ZAIKO_ENV=production`
- `ZAIKO_COOKIE_SECURE=true`
- DBとアップロード先は絶対パス
- `ZAIKO_SESSION_TTL`は5分以上168時間以下
- TLS終端とレート制限を行うリバースプロキシの背後で起動
- 外部公開するのはリバースプロキシだけとし、アプリは原則`127.0.0.1`で待受

起動時に未適用マイグレーションがトランザクション内で自動適用されます。起動前に必ずバックアップを取得してください。

## 日次バックアップ

サーバー稼働中でもSQLiteの整合したスナップショットを作成できます。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\backup.ps1 `
  -DatabasePath C:\ProgramData\ZaikoKanri\data\zaiko.db `
  -UploadDirectory C:\ProgramData\ZaikoKanri\uploads `
  -BackupDirectory D:\ZaikoBackups
```

ZIPには次を格納します。

- SQLiteスナップショット
- アップロード画像
- 作成日時、SHA-256、適用済みマイグレーションのマニフェスト

バックアップは別ディスクまたはオブジェクトストレージへ複製し、世代保持と暗号化は運用基盤側で設定してください。月1回以上、隔離環境で復元テストを行います。

## 整合性確認

```powershell
.\bin\zaiko-maintenance.exe verify `
  -database C:\ProgramData\ZaikoKanri\data\zaiko.db
```

`integrity=ok`、マイグレーション6件、`latest=000006_approvals`が成功条件です。

## 復元

復元はデータを差し替えるため、必ずメンテナンス時間を確保します。

1. 在庫管理サーバーを正常停止します。
2. 復元先DBの`-wal`・`-shm`が残っていないことを確認します。
3. 次を実行します。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\restore.ps1 `
  -ArchivePath D:\ZaikoBackups\zaiko-backup-YYYYMMDD-HHMMSS.zip `
  -DatabasePath C:\ProgramData\ZaikoKanri\data\zaiko.db `
  -UploadDirectory C:\ProgramData\ZaikoKanri\uploads `
  -Confirm RESTORE
```

復元前のDBと画像は`.pre-restore-日時`へ退避されます。復元スクリプトは、ZIP内DBのSHA-256、整合性、最新マイグレーションを検証してから差し替えます。

## ロールバック

アプリ更新のロールバックは次の順で行います。

1. サービス停止
2. 直前バイナリへ戻す
3. DB変更を伴うリリースの場合は、直前バックアップから復元
4. 保守ツールの`verify`を実行
5. サービス起動後に`/healthz`と主要業務画面を確認

Downマイグレーションを本番データへ直接適用する方法は、後続フェーズのデータ消失を招くため標準手順にしません。DBロールバックはバックアップ復元を使用します。

## 障害時の採取情報

- 発生日時と利用者
- 画面に表示された操作内容
- HTTPレスポンスの`X-Request-ID`
- 該当時刻のアプリログ
- 監査ログのリクエストID
- `zaiko-maintenance verify`の結果

パスワード、Cookie、CSRFトークン、個人情報をチケットやチャットへ貼り付けないでください。
