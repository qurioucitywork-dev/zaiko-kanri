# 別PC・別Codex向け引き継ぎプロンプト — 在庫管理ツール／空データ環境

作成日：2026-09-14（日本時間）

このMarkdownファイルを移行先PCのCodexに添付し、「このファイルの指示を実行してください」と依頼してください。ファイル全体を貼り付けても構いません。

## 移行元で先に確認すること（重要）

- GitHubリポジトリ名：**qurioucitywork-dev/zaiko-kanri**
- リポジトリURL：**https://github.com/qurioucitywork-dev/zaiko-kanri**
- 現在の作業ブランチ：**codex/react-gorm-aws-foundation**
- 調査時のローカルHEAD：`79ea01fc5db92ca0e7705ce5c5ada0a3e314a1fc`
- 上記HEADは変更前の参考情報であり、最新公開版を示すものではありません。この文書と同時に共有する機能コード・マイグレーション・テスト・公開用アセットを含むコミットを取得してください。移行元から通知されたpush済みコミット番号と、実際に取得したHEADを照合してください。
- リポジトリの共有先GitHubアカウントにアクセス権を付けてください。DBのバックアップや認証情報を共有する必要はありません。
- 未コミット変更は別PCから取得できません。「公開画面が更新されている」ことと「GitHubに最新コードがある」ことは別です。初版では未コミットの注意を記載していましたが、移行元のpush完了通知を受け取った後は、その通知のブランチ・コミットを使って再開してください。
- `git add .` で一括登録しないでください。現在の作業ツリーには帳票出力、画像、ログなどもあります。コード・必要な静的アセット・マイグレーション・テスト・文書だけを選び、機密や実データが含まれないことを確認してください。

ここから下が移行先Codexへの実行指示です。

---

## 1. あなたに実行してほしい作業

このPCに「在庫管理ツール（Zaiko Kanri）」を新規構築してください。説明文を返すだけで終わらず、環境調査、必要ツールの導入、Git取得、空のDB作成、初期管理者の作成、ビルド、起動、動作確認、Cloudflare一時URL発行、再起動用スクリプト作成まで実施してください。

ソースコードと機能は共有Gitリポジトリから引き継ぎます。**移行元のマスタデータ・商品・伝票・画像・取引履歴は一切引き継ぎません。** 機能や画面を削除するのではなく、業務データだけが空の新しい環境を作ってください。

OSを検出してください。WindowsではPowerShell＋Docker Desktop／WSL 2を基本とし、macOS／Linuxなら公式手順に合わせて同等の構成にしてください。移行元のユーザー名、OneDriveのパス、実行中プロセスID、ポート、既存の一時URLをそのまま流用しないでください。

必要なローカルのセットアップ用コード追加・修正もこの依頼に含みます。既存のセットアップ機能が不足していれば実装してテストしてください。アプリの業務仕様を作り直す依頼ではありません。

通常の安全なセットアップは進めて構いません。ただしGitHubへのログイン、アクセス権不足、OS管理者承認、ライセンス同意、再起動、秘密情報の入力など、本人の操作が必要なところは無理に回避せず短く案内してください。必要なら再開用ファイルを残してください。公開開始前には、このPCの管理画面をインターネットへ公開することを説明し、利用中のツールが要求する確認を取ってください。

## 2. Gitの取得と最新版チェック

対象は次のリポジトリです。

```text
Repository: qurioucitywork-dev/zaiko-kanri
URL: https://github.com/qurioucitywork-dev/zaiko-kanri
Source branch: codex/react-gorm-aws-foundation
```

1. 作業フォルダーは、既存プロジェクトと衝突しない短いローカルパスを選んでください。Windowsの例は `C:\Work\zaiko-kanri`。可能なら同期フォルダーを避けてください。
2. 既に同名フォルダーがあれば `git remote -v`、`git status --short` を確認し、他人の変更を上書きしないでください。必要なら別フォルダーへcloneしてください。
3. 新規cloneの場合のコマンドは以下です。

```sh
git clone --branch codex/react-gorm-aws-foundation https://github.com/qurioucitywork-dev/zaiko-kanri.git zaiko-kanri
```

4. そのブランチがない場合は `git ls-remote --heads` で確認し、共有先の正しいブランチを利用者に確認してください。古いブランチやdefault branchへ無言で切り替えないでください。
5. 実際のcommit SHA・ブランチ・remote URLを記録してください。添付文書の古いHEADへresetしないでください。
6. `AGENTS.md`、`README.md`、`go.mod`、`Dockerfile`、`compose.yaml`、`frontend/package.json`、ロックファイル、`cmd/server/main.go`、`internal/config/config.go` を読んで現在のコードを優先してください。
7. 最新機能が共有されているか、少なくとも次をコードで確認してください。
   - `internal/persistence/sales_adjustments.go` があり、入力に `adjustmentDate` がある。
   - `internal/persistence/migrations/000083_sales_adjustment_line_directions.up.sql` がある。より新しい番号があることは問題ありません。
   - 管理画面の `openSalesChangeModal` がボタン押下日を保持する。
   - `toggleInventoryGrossMarginSort` と `inv-gross-margin-sort-btn` がある。
   - 売上修正伝票の消費税表示、原価管理、原価調整伝票機能が含まれる。
8. 欠けている場合は「移行元で最新変更が未pushの可能性がある」と報告してください。旧版のまま最新システムを移行できたと扱わないでください。ツール導入など独立して進められる準備は進めて構いません。

GitHubの権限は移行先で利用するGitHubアカウントで確認し、移行元のトークン・Cookie・SSH秘密鍵はコピーしないでください。セットアップ用修正は必要なら `codex/clean-pc-setup` などのローカルブランチで管理し、pushは別途依頼がない限り実行しないでください。

## 3. 空データの定義

最終的に次の業務レコードを0件にしてください。既存DBを削除して0件にするのではなく、**専用の新規DB・専用volume**で構築してください。

- 商品、パーツ、在庫イベント、商品画像・添付ファイル。
- 仕入、売上、売上修正、出荷、委託、仕入返品、売上返品、原価分け・原価合わせ・原価調整の各伝票と明細。
- 取引先、仕入先、販売先、ゲスト、架空の社員・バイヤー。
- ブランド、素材、駆動方式、コンディション、付属品、オークション、ベルト素材、文字盤、商品形状、マーキング、パーツ名称などの編集可能な業務マスタ。
- BOXの登録データ／割当、公開カタログ、購入依頼、通知、承認、棚卸、相場、為替履歴。
- 会社住所・銀行口座・適格請求書番号などのサンプル会社情報、過去の採番履歴。

アプリ稼働に必要な以下だけは例外として作成できます。

- DBスキーマ・マイグレーション履歴、ロール・権限定義などのシステム定義。
- 新規組織1件と初期管理者1名。必要な場合だけ、その管理者の社員プロフィール1件。
- 空の会社プロフィール、未設定のシステム設定。組織コードは例として `CLEAN`、組織名は「新規環境（未設定）」にして、移行元の会社情報を使わないこと。
- ログイン確認によって生成される新規セッション／監査ログ。これは既存業務データではありません。
- ステータスや通貨コード、入力選択肢などコードで定義される必須の列挙値は削除しないこと。為替の「数値履歴」は投入しないこと。

UIのBOXの空枠や、未登録を示す空欄は残してください。業務マスタが空で登録に必要な情報が足りなければ「先にマスタ登録が必要」と案内し、架空データで補わないでください。

次のファイルやデータを移行元からコピー・復元しないでください：`.data/`、`.backups/`、DB dump、Docker volume、uploads、`.env`、認証情報、`.runtime/`、ログ、実帳票PDF・CSV、ブラウザーlocalStorage。Gitで取得されるサンプル／テストフィクスチャも業務DBへ読み込まないでください。

## 4. このリポジトリ固有の注意点

2026-09-14時点でコードから確認した内容です。移行先で実コードを再確認してください。

1. **`ZAIKO_ENV=development` は使わないこと。** `cmd/server/main.go` がSQLite側で `SeedPreview`、`SeedInventoryPreview`、`SeedMarketPreview`、`SeedSalesPreview`、`SeedRequestPreview`、`SeedApprovalPreview` を、PostgreSQL側で `SeedPreviewIdentity`、`SeedPreviewMasters`、`SeedPreviewInventory` を呼び出します。DBを後から空にしても再起動で再投入されるおそれがあります。
2. 既存 `compose.yaml` はdevelopment、SQLiteを既定値とし、PostgreSQLにも固定の開発用パスワードを設定しています。**そのまま全サービスを起動しないこと。** 専用の独立Composeファイルを作成してください。
3. `scripts/bootstrap-other-pc.ps1` と `scripts/dev*.ps1` はプレビュー前提の箇所があります。前者にはライセンス自動同意フラグもあります。この空環境セットアップへそのまま流用せず、内容を確認して新しいスクリプトを作ってください。
4. **`cmd/maintenance bootstrap-admin` は現状SQLite専用です。** PostgreSQLを業務DBにした場合、これだけでは管理APIへログインできません。`internal/web/api_identity.go` はPostgreSQL使用時にPostgreSQL側の認証を使います。
5. PostgreSQL使用時でも `cmd/server/main.go` は互換用SQLiteを開いてマイグレーションします。こちらも新規の専用ファイルを使い、プレビュー投入をしないこと。
6. `frontend/public/admin-reference/js/data.js` などにデモ表示用データがあります。DBが空でもUIへサンプルが復活しないか確認してください。`api_bridge.js` のhydration、localStorage、会社情報・為替のfallbackも対象です。DB取得エラーを「空データ」として隠さずエラー表示にし、正常な空レスポンスは空として表示してください。
7. フロントの原本は `frontend/public/admin-reference/`、Goに組み込まれる配信アセットは `internal/web/react-dist/` です。`frontend/vite.config.js` のbuild先へ出力してからGoをbuildしてください。必要なら `internal/web/reference_admin_test.go` のハッシュと関連テストを更新してください。
8. `ZAIKO_ENV=production` は `ZAIKO_COOKIE_SECURE=true` とHTTPSの `ZAIKO_PUBLIC_BASE_URL` 等を要求します。ローカルHTTP準備と公開HTTPS稼働を分けてください。`production` の検証を無効化して起動するのは禁止です。

## 5. ツール導入

まず `git --version`、`docker version`、`docker compose version`、`go version`、`node --version`、`cloudflared --version` を確認し、不足分だけ導入してください。

推奨構成は **Docker上のPostgreSQL＋ホスト上のGoアプリ＋ホスト上のcloudflared** です。既存構成に近く、空環境の初期化とトンネル発行を切り分けやすくなります。

- Windows：Docker Desktop、WSL 2／Linuxコンテナー、Git、Go、Node.js、cloudflared。GitHub CLIは必要な場合のみ。
- Goの必要バージョンは `go.mod`（調査時 `go 1.26.0`）で確認。
- NodeとパッケージマネージャーはDockerfile・ロックファイルを確認。調査時DockerfileはNode 24／pnpm 11.9.0です。
- pnpm lockを使うなら指定バージョンで `pnpm install --frozen-lockfile`。npmを使う場合は `package-lock.json` の存在と整合性を確認して `npm ci`。無計画にlockを書き換えないこと。
- WSLの状態確認は `wsl --status`、`wsl --version`、`wsl -l -v`。必要時のWSL導入や再起動はユーザーへ案内し、再起動後に再開してください。
- Docker Desktopのインストール完了だけでなく `docker info` が成功することを確認してください。

Windowsで利用できる公式パッケージIDの候補です。実行前に `winget show --id ... --exact` で提供元を確認し、利用条件への同意が必要ならユーザーに判断してもらってください。

```powershell
winget install --id Git.Git --exact
winget install --id Docker.DockerDesktop --exact
winget install --id GoLang.Go --exact
winget install --id OpenJS.NodeJS.LTS --exact
winget install --id Cloudflare.cloudflared --exact
# GitHub CLIが必要な場合のみ
winget install --id GitHub.cli --exact
```

macOS／Linuxでは同じツールの公式手順を使い、Windowsコマンドを実行しないでください。未知の配布元のインストーラー、安全機能の無効化、TLS検証の無効化は使わないでください。

## 6. 専用PostgreSQLの作成

移行先に次の成果物を作成してください。ここに挙げる新しいファイルやコマンドは**既存実装ではなく、あなたが必要に応じて作成するもの**です。

- `compose.clean.yaml`：この環境専用のPostgreSQL。
- `.env.clean.local`：新規DBの接続情報など。Git追跡対象外。
- `scripts/setup-clean.ps1`：事前確認、build、初期化。再実行で既存業務データを消さない。
- `scripts/start-clean.ps1`：専用設定でGoアプリを起動。
- `scripts/start-clean-tunnel.ps1`：トンネルの起動、URL取得、必要なアプリ再起動。
- `scripts/stop-clean.ps1`：この環境のアプリとトンネルだけ停止。DB volumeは残す。
- `docs/clean-environment-runbook.md`：運用／再開手順と確認結果。

macOS／Linuxなら同等のshellスクリプトにして構いません。

Composeは既存ファイルとマージせず、`-f compose.clean.yaml -p zaiko-clean` を常に指定してください。既に同じprojectやvolumeがある場合は中身・所有関係を確認し、別環境なら新しいproject名にしてください。

```yaml
# 例。ポート使用状況を確認してから採用すること。
services:
  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_DB: zaiko_clean
      POSTGRES_USER: zaiko_clean
      POSTGRES_PASSWORD: ${ZAIKO_CLEAN_DB_PASSWORD:?新規DBパスワードが必要}
    ports:
      - "127.0.0.1:15432:5432"
    volumes:
      - postgres-clean-data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U zaiko_clean -d zaiko_clean"]
      interval: 5s
      timeout: 3s
      retries: 20
volumes:
  postgres-clean-data:
```

```sh
docker compose --env-file .env.clean.local -f compose.clean.yaml -p zaiko-clean up -d postgres
docker compose --env-file .env.clean.local -f compose.clean.yaml -p zaiko-clean ps
```

DBのパスワードは移行先で暗号学的に安全な方法で新規生成し、URLに安全な文字列または適切なURLエンコードを使用してください。平文秘密をGit・文書・通常ログへ残さず、保存先にはユーザー専用の権限を設定してください。`docker compose config` は秘密値を展開するため、その出力をチャットへ貼らないでください。

`docker system prune`、`docker volume prune`、`docker compose down -v`、既存DBのdrop/truncate、移行元DBへの接続は禁止です。今回作成したDBにもテスト業務データを常設しないでください。

## 7. サンプルなしの初期管理者作成を実装する

既存コードにPostgreSQL対応のクリーン初期化があれば監査して再利用してください。なければ `cmd/bootstrap-clean` 等のCLIとrepository処理を実装してください。

要件：

1. 対象接続先・DB名・組織コードを事前表示。ただし接続パスワードは表示しない。
2. PostgreSQLのマイグレーション／必要なcore schemaを適用する。migration内のデータ投入も確認する。
3. **新規・空DBに限り**、新組織、初期管理者、role／permission、必要な空設定のみをtransactionで作成する。
4. `SeedPreviewIdentity` をそのまま呼ばない。これは架空社員・会社・銀行口座も作成する。必要な権限定義とHashPasswordなど、データ非依存の処理だけを再利用する。
5. 初期管理者用パスワードは安全に新規生成または本人から安全に受け取り、既存のbcrypt実装でhash化する。preview用既知パスワードは使用しない。本人が確認できる保護されたローカルファイル等で受け渡す。
6. 必要なら管理者のstaff profileだけを例外作成し、その理由を記録する。架空のバイヤーは作らない。
7. 会社プロフィールは未設定で作り、会社取得APIが500にならないようにする。サンプル住所・口座などで埋めない。
8. 同じ環境への再実行では正常に「初期化済み」と判定するか安全に拒否する。既存のpassword・権限・業務データを上書きしない。
9. 管理API用PostgreSQL認証と、互換SQLiteの扱いを区別する。通常入口は `/app/admin-reference/index.html` → `/api/v1/auth/login`。互換画面のためだけにサンプルユーザーを追加しない。
10. パスワード・業務データを含まないテストを用意し、初期化後／再実行後とも業務テーブル0件を確認する。

既存のアプリ実行ファイルとは別にこのCLIをbuildして実行してください。利用していないDBまで初期化しないこと。

## 8. ローカルの起動と空状態確認

まずループバックだけで起動してください。例は次のとおりです。環境変数の読み込みは起動スクリプトで明示的に実装し、`.env`をGoが自動で読むと仮定しないこと。

```text
ZAIKO_ENV=test
ZAIKO_ADDRESS=127.0.0.1:18088
ZAIKO_DATABASE_DRIVER=postgres
ZAIKO_DATABASE_URL=postgres://zaiko_clean:<移行先で新規生成した値>@127.0.0.1:15432/zaiko_clean?sslmode=disable
ZAIKO_DATABASE_PATH=<新規の専用絶対パス>/compat.db
ZAIKO_UPLOAD_DIRECTORY=<新規の専用絶対パス>/uploads
ZAIKO_STORAGE_DRIVER=local
ZAIKO_ORGANIZATION_CODE=CLEAN
ZAIKO_COOKIE_SECURE=false
ZAIKO_PUBLIC_BASE_URL=http://127.0.0.1:18088
```

`test` はこのコードでサンプル投入を避けてローカルHTTPを確認するための準備設定です。公開時には後述のproduction／Secure Cookieへ切り替えます。既存の開発設定を継承しないでください。ポートが使用中ならこの環境の空きポートを選び、後続手順を連動させてください。

ビルドの流れ：ロックに合ったフロント依存取得 → frontend build → frontendテスト → Goテスト → Go build。通常起動用exeは `.runtime/` の新しい名前に出力し、稼働中の他環境のexeを上書きしないでください。

テストは独立した一時DB／テスト用volumeで行ってください。`go test ./...` の前に各テストが参照する環境変数・DB接続先を確認し、構築した運用DBをテスト先にしないでください。終了後、運用DBの件数が0のままか再確認してください。

確認対象：

- `/healthz`、`/readyz`、`/api/v1/health` が正常、DB driverがpostgres。
- 初期管理者でログイン成功。ログアウト状態では保護APIが401/403になる。
- `/api/v1/products?includeCancelled=true`、`/api/v1/purchases`、`/api/v1/sales`、`/api/v1/cost-adjustments`、各業務マスタAPIなどが正常な空レスポンス。
- 管理画面、在庫一覧、伝票一覧、原価管理、マスタ画面がエラーなく開き、架空データがない。
- マスタ未設定、会社情報未設定、為替未設定を正常表示し、既定の架空為替レート・サンプル口座を現在値として出さない。未設定で計算できない箇所は未設定と案内する。
- 管理者1名などの許可した例外以外は業務データが0件。DBの実テーブルを列挙し、除外理由付きで件数レポートを残す。
- アプリとDBを通常再起動してもサンプルデータが増えない。
- 最新の粗利率ソート、売上修正伝票のボタン押下日、課税表示などの回帰テストが通る。操作テスト用商品が必要なら別の使い捨てテストDBだけで作成する。

空画面にするためにAPIエラーを隠す実装は禁止です。テーブル不存在／認証不整合／migration失敗を直してください。

## 9. Cloudflare一時URLの発行

今回作成したアプリのHTTPポートだけを公開します。PostgreSQL、Docker API、管理者作成CLI、ファイルサーバーは公開しないでください。Cloudflareの追加ログインがないQuick Tunnelでも、**アプリ自身のログインは必ず残す**こと。

ホストにcloudflaredを導入し、次の形式で起動してください。

```sh
cloudflared tunnel --url http://127.0.0.1:18088
```

Cloudflareのアカウント／独自ドメインの登録を必要としないQuick Tunnelです。標準出力と標準エラーを専用ログへ記録し、実際に出力された `https://<ランダム>.trycloudflare.com` を取得してください。URLを推測したり、移行元のURLを再利用したりしないこと。公式手順：[Cloudflare Quick Tunnels](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/)。

トンネル公開前に、未ログインで保護APIを読めないことをローカルで確認してください。URL発行後、トンネルを維持したままアプリだけを次の設定で再起動してください。

```text
ZAIKO_ENV=production
ZAIKO_COOKIE_SECURE=true
ZAIKO_PUBLIC_BASE_URL=https://<実際に発行されたホスト名>
```

その他のDB・組織・アップロード先は同じ専用設定を保持します。トンネル内のoriginはループバックHTTPのままで構いません。公開HTTPS URLでログインし、Cookie・CSRF・API通信まで確認してください。Secure Cookieへ切り替えた後はローカルHTTPでログイン確認を続けないこと。

公開する入口：

```text
https://<実際に発行されたホスト名>/app/admin-reference/index.html
https://<実際に発行されたホスト名>/app/admin-reference/app.html
```

注意事項：

- Quick Tunnelは開発・検証用で、安定稼働を保証する本番ホスティングではありません。PCの電源／スリープ、アプリ・Docker・cloudflared停止で利用できなくなります。実業務公開を継続する際は恒久的なホスティングとアクセス制御を別途設計してください。
- 再起動でURLが変わる場合があります。そのたびに `ZAIKO_PUBLIC_BASE_URL` と運用メモを更新し、アプリを再起動してください。
- 既存の `.cloudflared/config.yaml` などがQuick Tunnelと干渉する場合、既存設定を勝手に削除・上書きしないでください。公式手順で隔離可能な実行方法を確認するか、所有者の了承を得た退避・復元を行ってください。
- Windowsでバックグラウンド起動する場合は `Start-Process -WindowStyle Hidden` を使い、PID・exeの絶対パス・ログを記録してください。終了時は今回起動した正確なPID／コマンドだけを対象にしてください。
- TLS、認証、ファイアウォールを無効化して接続エラーを回避しないでください。必要な通信条件は最新の公式ドキュメントで確認してください。

## 10. 再実行・中断からの復旧

セットアップの進捗を `docs/clean-setup-status.md` に保存してください。秘密値は含めず、完了段階・実際のパス・Docker project名・volume名・commit SHA・未完了項目を記載してください。

再実行時にはこのファイルと実環境を照合し、完了済みのインストール、DB作成、管理者作成を不用意にやり直さないでください。ユーザーが後で入力したデータを「空に戻す」動作は禁止です。

起動・停止・再開手順には次を明記してください。

- Docker Desktop起動 → 専用PostgreSQL起動 → アプリ起動 → 必要時Quick Tunnel発行。
- アプリとトンネルのPID／ログの場所、専用volumeの名前。
- URLが変わったときの設定更新方法。
- 通常停止ではDB volume／uploadsを削除しないこと。
- ビルド更新時は既存業務DBを初期化しないこと。

## 11. 完了条件と報告

以下がすべて完了するまで、完了したと報告しないでください。

- 共有された最新コードの取得とバージョン確認。
- Docker／PostgreSQL、Go／フロントbuild、必要ツールが実際に動作。
- 新規管理者でログイン可能、未ログインの保護APIは拒否。
- 商品・伝票・業務マスタは0件、必要なシステム定義の例外だけ存在。
- UIにもデモデータがなく、再起動しても混入しない。
- 新しいHTTPS一時URLで画面とAPIが利用可能。
- 再起動用スクリプト・運用手順・検証結果が保存済み。

最終報告には、リポジトリ名、実際のbranch／commit SHA、作業フォルダー、利用したバージョン、ログインURL、初期管理者名と秘密情報の安全な確認場所、空データ件数結果、起動／停止方法、変更したセットアップファイル、残る制約を書いてください。パスワードを公開ログやGitへ出さないでください。

権限不足などで完了できない場合は、できた範囲と必要な本人操作を具体的に報告し、最新データ・公開動作・空状態を確認できたと偽らないでください。

## 12. 公式資料と参照コード

導入時の最新要件は次の公式資料を確認してください。

- [Docker Desktop：Windows導入](https://docs.docker.com/desktop/setup/install/windows-install/) — WSLや初回起動、ライセンス条件は導入先で確認。
- [Docker Desktop：WSL 2](https://docs.docker.com/desktop/features/wsl/) — Docker engineの動作確認まで行う。
- [Docker Engine：Linux導入](https://docs.docker.com/engine/install/)
- [Cloudflare Quick Tunnels](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/) — 一時URLの発行方法・制限を確認。
- [cloudflared公式リリース](https://github.com/cloudflare/cloudflared/releases)
- [Go公式ダウンロード](https://go.dev/dl/)
- [Node.js公式ダウンロード](https://nodejs.org/en/download)
- [Git公式ダウンロード](https://git-scm.com/downloads)

リポジトリ内の主な確認先：`cmd/server/main.go`、`cmd/maintenance/main.go`、`internal/config/config.go`、`internal/persistence/identity_seed.go`、`internal/persistence/identity.go`、`internal/persistence/migrations/`、`internal/web/api_identity.go`、`frontend/public/admin-reference/js/api_bridge.js`、`frontend/public/admin-reference/js/data.js`、`frontend/vite.config.js`、`frontend/tests/reference-admin.test.mjs`、`internal/web/reference_admin_test.go`。

**最重要：この依頼は「コードと機能の引き継ぎ＋業務データが空の新規環境構築」です。移行元DBの移行、現行システムの停止、既存データの削除、既存認証の共有ではありません。**
