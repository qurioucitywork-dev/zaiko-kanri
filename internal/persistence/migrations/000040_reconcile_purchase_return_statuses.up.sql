-- Purchase-return slips are the source of truth for the physical inventory
-- status.  Before a tracking number is saved the item is still on hand and is
-- counted as 仕入返品.  Once tracking is saved the return has been shipped and
-- the item becomes 取消.
WITH desired_status AS (
    SELECT
        line.product_id,
        product.organization_id,
        product.inventory_status AS previous_status,
        CASE
            WHEN BOOL_OR(BTRIM(COALESCE(slip.tracking_number, '')) <> '') THEN 'cancelled'
            ELSE 'return_pending'
        END AS next_status,
        (ARRAY_AGG(slip.created_by ORDER BY slip.created_at DESC))[1] AS actor_user_id
    FROM return_slips AS slip
    JOIN return_lines AS line ON line.return_slip_id = slip.id
    JOIN products AS product ON product.id = line.product_id
    WHERE slip.operation_type = 'purchase_return'
      AND slip.status <> 'cancelled'
      AND product.deleted_at IS NULL
    GROUP BY line.product_id, product.organization_id, product.inventory_status
), changed AS (
    UPDATE products AS product
    SET
        inventory_status = desired.next_status,
        cancelled_at = CASE
            WHEN desired.next_status = 'cancelled' THEN COALESCE(product.cancelled_at, NOW())
            ELSE NULL
        END,
        cancelled_by = CASE
            WHEN desired.next_status = 'cancelled' THEN COALESCE(product.cancelled_by, desired.actor_user_id)
            ELSE NULL
        END,
        cancel_reason = CASE
            WHEN desired.next_status = 'cancelled' THEN
                CASE
                    WHEN BTRIM(product.cancel_reason) = '' THEN '仕入返品の配送番号保存'
                    ELSE product.cancel_reason
                END
            ELSE ''
        END,
        updated_at = NOW()
    FROM desired_status AS desired
    WHERE product.id = desired.product_id
      AND product.inventory_status IS DISTINCT FROM desired.next_status
    RETURNING
        product.id,
        product.organization_id,
        desired.previous_status,
        desired.next_status,
        desired.actor_user_id
)
INSERT INTO inventory_events (
    id,
    organization_id,
    product_id,
    event_type,
    from_status,
    to_status,
    reason,
    actor_user_id,
    created_at
)
SELECT
    'evt_mig040_' || MD5(changed.id || ':' || changed.next_status),
    changed.organization_id,
    changed.id,
    'purchase_return_status_reconciled',
    changed.previous_status,
    changed.next_status,
    CASE
        WHEN changed.next_status = 'cancelled' THEN '配送番号保存済みの仕入返品伝票に基づく取消'
        ELSE '配送番号未保存の仕入返品伝票に基づく仕入返品'
    END,
    changed.actor_user_id,
    NOW()
FROM changed
ON CONFLICT (id) DO NOTHING;
