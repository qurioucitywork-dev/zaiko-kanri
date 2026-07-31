# D1 オブジェクトメタデータ書込み

## 1. 対象

`dataaccess.ObjectMetadataWriter` の4操作を、Containerから内部D1 Workerへ送る
単一HTTPリクエストと、D1の単一`batch`書込み境界で実装する。

- `CreatePendingObject`
- `MarkObjectReady`
- `MarkObjectFailed`
- `MarkObjectDeleted`

DBとR2を同一トランザクションとして扱わない。上位の`objectservice`が
`pending -> R2 Put -> Head照合 -> ready/failed`を調整する。

## 2. テーブル

`0002_object_metadata.sql`は次を追加する。

- `object_metadata`: プロバイダー非依存メタデータ
- `object_metadata_idempotency`: tenant、操作、冪等性キー単位の実行記録
- lifecycle遷移を制限するトリガー

bucket、object key、region、version、署名付きURL、プロバイダーrequest ID、
credentialは保存しない。呼出し側が扱うロケーターは不透明な`object_id`だけである。

`object_metadata`の主キーは`(organization_id, id)`とし、同じIDを別tenantで
安全に利用できる。全ての参照・更新SQLは`organization_id`を必須条件とする。
別tenantに存在するIDは、存在しないIDと同じ`not_found`になる。

`product_id`、`created_by`、冪等性記録の`actor_id`は、IDだけではなく
`organization_id`との複合外部キーで親テーブルを参照する。SQLiteの複合外部キー要件を
満たすため、既存のグローバル主キーは変更せず、`products(organization_id, id)`と
`users(organization_id, id)`に一意インデックスを追加する。これにより、アプリケーションの
事前検証を迂回した書込みでも、別tenantのproduct/userをDBが拒否する。

## 3. lifecycle

許可する遷移は次の通り。

```text
pending -> ready
pending -> failed
pending -> deleted
ready   -> deleted
failed  -> deleted
```

`ready_at`はreadyからdeletedになった後も履歴として保持する。ready時のchecksumと
sizeも削除せず、照合・監査に利用できる状態を維持する。deletedからの再遷移は禁止する。

Workerは更新SQLにもstatus条件を含める。トリガーは、将来別経路から更新された場合の
二重防御である。

## 4. 内部HTTP契約

| 操作 | Method / Path |
|---|---|
| pending作成 | `POST /internal/v1/object-metadata/pending` |
| ready | `POST /internal/v1/object-metadata/{id}/ready` |
| failed | `POST /internal/v1/object-metadata/{id}/failed` |
| deleted | `POST /internal/v1/object-metadata/{id}/deleted` |

必須ヘッダー：

- `x-zaiko-tenant-id`
- `x-zaiko-idempotency-key`
- `x-zaiko-canonical-hash`
- `content-type: application/json`

`actor_id`と`requested_at`はJSON本文に含める。ただしcanonical hashはtransport
retryで変化し得る`requested_at`だけを除外し、次の正規化JSONをSHA-256へ入力する。

- operation name
- trim済みtenant ID
- trim済みactor ID
- `actor_id`と`requested_at`を除いた操作固有のbusiness payload

tenant、actor、operation、business payloadのいずれかが異なればhashも異なる。
同一tenant・actor・idempotency key・business payloadを再送する際は、
`requested_at`が新しいretry時刻でも同じhashとなる。`ready_at`や`deleted_at`など
業務上の日時は除外せず、business payloadとして束縛する。

Goクライアントがヘッダーへ設定したhashをWorkerが同じ規則で再計算して照合する。
ヘッダー値を信用しただけの冪等性判定は行わない。本文の`requested_at`はhashから
除外しても、初回処理の作成・更新・監査時刻として引き続き保存する。

## 5. 冪等性とbatch境界

冪等性の一意キーは次である。

```text
(organization_id, operation_name, idempotency_key)
```

- 同じキー・同じcanonical hashは保存済み応答を再生する
- 同じキー・異なるhashは`idempotency_mismatch`
- business更新と冪等性記録は同じ`DB.batch`へ入れる
- transition更新では、直前のUPDATEが1行変更した場合だけ冪等性記録をINSERTする
- 競合時は冪等性記録を再読込し、同一要求の並行実行なら再生、それ以外は`conflict`

これにより、更新だけ成功して冪等性記録が欠ける部分成功と、その逆を防ぐ。
Workerの1回の`fetch`中で完結し、ContainerとWorkerをまたぐ長時間トランザクションは作らない。

## 6. 検証とエラー

Workerはtenant、actor、product、object ID、MIME、checksum、size、日時、
reason codeを検証する。actorとproductは同一tenant内の有効レコードだけを許可する。

object metadataの全書込みでは、actorについて次を確認する。

- 所属する`organizations.is_active = 1`
- `users.is_active = 1`
- `users.deleted_at IS NULL`
- 個別`user_permissions`が存在する場合はその`inventory.write`の効果を優先する
- 個別設定がない場合だけ、`role_permissions`の`inventory.write`付与を使用する

したがって個別`deny`はロール付与より優先され、個別`allow`またはロール付与のどちらも
ない場合は拒否する。新しいロール名やpermission keyは追加しない。actorの存在・所属・
権限状態を外部へ漏らさないため、停止組織、無効・削除済み・権限不足・別tenantのactorは
いずれも既存契約どおり`not_found`で返す。この検証は冪等性応答の再生より先に行い、
組織停止または権限取消後のactorへ過去の成功応答を返さない。

| Worker code | HTTP | Go error |
|---|---:|---|
| `invalid_argument` | 400 | `ErrInvalidArgument` |
| `not_found` | 404 | `ErrNotFound` |
| `conflict` | 409 | `ErrConflict` |
| `idempotency_mismatch` | 409 | `ErrIdempotencyMismatch` |

Worker例外はSQL、binding、内部URL、credentialを含めず`service_unavailable`へ変換する。
Goクライアントも応答本文やtransport errorをそのまま上位へ返さない。
contextの取消・期限切れだけは標準context errorとして維持する。

## 7. READ統合とlegacy移行

`GET /internal/v1/products/{id}/objects`と`GET /internal/v1/objects/{id}`は、
新しい`object_metadata`を主READ元にする。これによりpending作成直後、ready化直後、
failed/deleted後もwriterとreaderが同じ状態を返す。

既存の`product_images`は移行期間中のfallback READとして残す。

- 同じtenant・IDが`object_metadata`にあれば新テーブルを優先
- 新テーブルにないlegacy画像だけ`ready`として合成
- tenant条件は新旧両方に適用

本移行期間中に新規書込みを`product_images`へ二重書込みしない。完全移行時は別作業で
legacy行を`object_metadata`へbackfillし、件数・tenant・product・sizeを照合してから
fallbackを削除する。

## 8. `index.ts`統合

`index.ts`はtenant検証後に`handleObjectMetadataRequest`を呼ぶ。モジュールが
対象POST pathを処理した場合はそのResponseを返し、それ以外は既存GETルーティングへ進む。
内部hop検証は従来どおり`index.ts`が先に実施する。

## 9. テスト

- Goクライアントの4操作、path、tenant/idempotency/hashヘッダー
- exact body SHA-256
- DTO変換
- エラー変換と機密応答の非露出
- 不正入力時のHTTP未送信
- migration適用
- pendingからready/failed、ready/failedからdeleted
- deleted後の再遷移拒否
- readyからdeleted後の`ready_at`保持
- TypeScript strict typecheck

外部D1/R2への接続は行わない。
