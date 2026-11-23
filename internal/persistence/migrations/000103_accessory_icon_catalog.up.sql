-- Populate the requested accessory catalog without renaming existing records.
DO $$
DECLARE org RECORD; entry RECORD; existing_id TEXT; next_code INTEGER;
BEGIN
  FOR org IN SELECT DISTINCT organization_id FROM accessories LOOP
    FOR entry IN SELECT * FROM (VALUES
      ('P','Guarantee(P)','紙ギャラ','','Guarantee(P)',1),
      ('C','Guarantee(C)','前期カードギャラ','','Guarantee(C)',2),
      ('NC','Guarantee(NC)','中期カードギャラ','月','Guarantee(NC)',3),
      ('NNC','Guarantee(NNC)','後期カードギャラ','年／月','Guarantee(NNC)',4),
      ('SV','Service Guarantee','サービスギャランティ','','Guarantee(SVC)|Guarantee(SV)|Service Guarantee',5),
      ('X','Box','箱（ボックス）','','Box',6),
      ('I','Inner Case','ケース（内箱）','','Inner Case|Case',7),
      ('A','Archive','アーカイブ','','Archive',8),
      ('F','Certificate','サーティフィケイト','','Certificate',9),
      ('D','Dial','文字盤','','Dial',10),
      ('Z','Bezel','ベゼル','','Bezel',11),
      ('B','Bracelet','ブレスレット','','Bracelet',12),
      ('L','Strap','ベルト','','Strap|Belt',13),
      ('OTH','Other','その他','','Other',14),
      ('KOMA','Bracelet Links','コマ（コマの数）','数字','Bracelet Links|Bracelet Parts',15)
    ) AS catalog(symbol,name,meaning,supplement,aliases,position) LOOP
      SELECT id INTO existing_id FROM accessories
      WHERE organization_id=org.organization_id AND is_active
        AND (lower(name)=ANY(string_to_array(lower(entry.aliases),'|')) OR meaning=entry.meaning
          OR (entry.symbol='SV' AND meaning='サービスギャラ'))
      ORDER BY sort_order,code LIMIT 1;
      IF existing_id IS NOT NULL THEN
        UPDATE accessories SET symbol=entry.symbol,supplement=entry.supplement,updated_at=now()
        WHERE id=existing_id;
      ELSE
        SELECT COALESCE(MAX(code::INTEGER),0)+1 INTO next_code FROM accessories
        WHERE organization_id=org.organization_id AND code ~ '^[0-9]{1,4}$';
        IF next_code>9999 THEN RAISE EXCEPTION 'Accessory master code capacity exceeded'; END IF;
        INSERT INTO accessories(id,organization_id,code,name,meaning,symbol,supplement,is_active,sort_order,created_at,updated_at)
        VALUES ('mst_acc_'||md5(org.organization_id||':'||entry.symbol),org.organization_id,lpad(next_code::TEXT,4,'0'),entry.name,entry.meaning,entry.symbol,entry.supplement,TRUE,
          (SELECT COALESCE(MAX(sort_order),0)+10 FROM accessories WHERE organization_id=org.organization_id),now(),now());
      END IF;
    END LOOP;
  END LOOP;
END $$;
