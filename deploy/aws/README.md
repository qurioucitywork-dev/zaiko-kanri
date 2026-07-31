# AWS 最終本番引継ぎ

最終本番は AWS を正とし、Cloudflare D1 / R2 / Containers は検証専用とする。

## 目標構成

- Go: ECS Fargate（最低 2 task、複数 AZ）
- DB: RDS for PostgreSQL（Multi-AZ、暗号化、削除保護、PITR）
- Object: S3（private、SSE、Versioning、lifecycle）
- Entry: ALB。必要に応じて CloudFront + WAF
- Secret: Secrets Manager
- Non-secret config: SSM Parameter Store
- Image: ECR、digest 固定
- Logs/Metrics: CloudWatch

## 本番切替前の必須ゲート

1. PostgreSQL adapter が SQLite / D1 と同じ contract test を通る。
2. 金額は `int64 + currency`、日時は UTC、業務日付は `YYYY-MM-DD` で損失なく往復する。
3. organization 境界、NotFound collapse、認可、冪等性、競合、rollback を確認する。
4. migration は単一の ECS migration job として、application rollout 前に一度だけ実行する。
5. RDS backup / restore と PITR を staging で実地確認する。
6. S3 Versioning、restore、孤児/missing object reconciliation を staging で実地確認する。
7. SQLite -> PostgreSQL は一方向移行とし、dual-write や暗黙 fallback を行わない。
8. read-only window、切替判定、rollback 判定、実施責任者を runbook で確定する。
9. 旧 SQLite は暗号化して保持し、復旧確認完了前に削除しない。

## ECS タスク定義

`ecs-task-definition.example.json` は secret や実 ARN を含まない構成例である。実際の値は
IaC と Secrets Manager / SSM から投入する。application task の起動時に migration を実行しない。

## データ移行順序

1. 現行 SQLite を read-only backup。
2. migration job で PostgreSQL schema を適用。
3. 匿名化 staging data を投入し contract / reconciliation を実行。
4. 本番 maintenance window で書込みを停止。
5. SQLite から PostgreSQL へ一方向 import。
6. 件数、金額、参照、状態、採番、承認、返品、棚卸を照合。
7. ECS new revision を deploy し readiness を確認。
8. ALB target を切替。
9. 監視し、判定条件を満たさなければ旧 revision へ戻す。

Cloudflare 検証データを本番へ昇格しない。D1 を本番正本または中継 DB として使用しない。
