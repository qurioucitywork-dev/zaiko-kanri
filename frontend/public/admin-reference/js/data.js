// =====================================================
// 在庫管理ツール - 共通データ・ユーティリティ
// =====================================================

// 売価はUSDで統一する。既存の円建てサンプルは、画面内の手入力為替レートと
// 同じ 1 USD = 155 JPY で初回ロード時に換算する。
const SALE_PRICE_CURRENCY = "USD";
const SALE_PRICE_JPY_PER_USD = 155;

const APP_DATA = {
  // マスタデータ
  brandRecords: [
    { code: "BRD-001", name: "ロレックス" },
    { code: "BRD-002", name: "オメガ" },
    { code: "BRD-003", name: "パテック・フィリップ" },
    { code: "BRD-004", name: "カルティエ" },
    { code: "BRD-005", name: "IWC" },
    { code: "BRD-006", name: "ブライトリング" },
    { code: "BRD-007", name: "タグ・ホイヤー" },
    { code: "BRD-008", name: "セイコー" },
    { code: "BRD-009", name: "グランドセイコー" },
    { code: "BRD-010", name: "その他" },
  ],
  auctionRecords: [
    { code: "AUC-001", name: "東京オークション" },
    { code: "AUC-002", name: "ワーカーオークション" },
    { code: "AUC-003", name: "APIオークション" },
    { code: "AUC-004", name: "その他" },
  ],
  // 名称配列は既存画面との互換用。固定IDは brandRecords を正とする。
  brands: ["ロレックス", "オメガ", "パテック・フィリップ", "カルティエ", "IWC", "ブライトリング", "タグ・ホイヤー", "セイコー", "グランドセイコー", "その他"],
  suppliers: [
    { code: "S001", name: "田中商事", address: "東京都台東区", contact: "03-1234-5678", invoice: "T0001234567890" },
    { code: "S002", name: "山田時計店", address: "大阪府大阪市", contact: "06-9876-5432", invoice: "T0000987654321" },
    { code: "S003", name: "ゴールデンウォッチ", address: "神奈川県横浜市", contact: "045-111-2222", invoice: "T0001122334455" },
    { code: "S004", name: "プレシャスメタル", address: "愛知県名古屋市", contact: "052-333-4444", invoice: "T0005566778899" },
    { code: "S005", name: "レアウォッチジャパン", address: "東京都港区", contact: "03-5555-6666", invoice: "T0009988776655" },
    { code: "S006", name: "クロノス東京株式会社", address: "東京都新宿区西新宿2-1-1", contact: "03-9999-0000", invoice: "T0007777888899" },
  ],
  buyers: [
    { code: "B001", name: "ウォッチマート", address: "東京都渋谷区", contact: "03-7777-8888", invoice: "T0001111222233", guestManaged: true },
    { code: "B002", name: "タイムレス商会", address: "福岡県福岡市", contact: "092-444-5555", invoice: "T0003333444455", guestManaged: true },
    { code: "B003", name: "ラグジュアリーアイランド", address: "沖縄県那覇市", contact: "098-666-7777", invoice: "T0005555666677", guestManaged: true },
    { code: "B004", name: "クロノス東京", address: "東京都新宿区", contact: "03-9999-0000", invoice: "T0007777888899", guestManaged: true },
  ],
  staff: ["山本 太郎", "佐藤 花子", "鈴木 一郎", "田中 美香", "伊藤 健司"],
  materials: [
    { code: "MAT-001", name: "ステンレスSS" },
    { code: "MAT-002", name: "イエローゴールドYG" },
    { code: "MAT-003", name: "ホワイトゴールドWG" },
    { code: "MAT-004", name: "ピンクゴールドPG" },
    { code: "MAT-005", name: "プラチナPT" },
    { code: "MAT-006", name: "チタンTi" },
  ],
  movements: [
    { code: "MOV-001", name: "自動巻き" },
    { code: "MOV-002", name: "手巻き" },
    { code: "MOV-003", name: "クオーツ" },
    { code: "MOV-004", name: "電波" },
    { code: "MOV-005", name: "スマート" },
  ],
  beltMaterialRecords: [
    { code: "BLT-001", name: "ステンレス" },
    { code: "BLT-002", name: "レザー" },
    { code: "BLT-003", name: "ラバー" },
    { code: "BLT-004", name: "チタン" },
    { code: "BLT-005", name: "ナイロン" },
  ],
  dialRecords: [
    { code: "DIA-001", name: "ブラック" },
    { code: "DIA-002", name: "ホワイト" },
    { code: "DIA-003", name: "シルバー" },
    { code: "DIA-004", name: "ブルー" },
    { code: "DIA-005", name: "グリーン" },
  ],
  accessories: ["BOX", "CASE", "GUARANTEE", "BRACELET PARTS", "CERTIFICATE", "ARCHIVE"],
  conditions: [
    { code: "CON-001", name: "未使用品 (N)" },
    { code: "CON-002", name: "未使用展示品 (N-)" },
    { code: "CON-003", name: "極美品 (S)" },
    { code: "CON-004", name: "美品 (A)" },
    { code: "CON-005", name: "良品 (AB)" },
    { code: "CON-006", name: "可品 (B)" },
    { code: "CON-007", name: "傷あり (BC)" },
  ],

  // サンプル在庫データ
  inventory: [
    {
      code: "20260301001", brand: "ロレックス", brandEn: "Rolex", model: "サブマリーナ", modelEn: "Submariner",
      ref: "116610LN", serial: "ZX123456", supplier: "S001", staff: "山本 太郎",
      purchasePrice: 850000, salePrice: 1180000, purchaseDate: "2026-03-01", status: "在庫中",
      material: "MAT-001", movement: "MOV-001", condition: "CON-003", accessories: ["BOX", "GUARANTEE"],
      boxNo: 1,
      images: [
        "https://sspark.genspark.ai/cfimages?u1=DgG%2FT%2FRiDi119SYIwu0kNOmGnlMd2p6TvXoiRTIyu5aLuI7OGoVdX7Vm5NOBuuyHc02AW9wpJ3XI7O19LikN1pjtVJF5dwqWH7CvGI0aacVG8RFyulGV1ez9EjAOUNqQdE0Pg2Qxaww3NmYj38YAEL0DGbqW6NB29GB5t7hb67g%3D&u2=%2FEq%2BGAMH3r7lDW2h&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=xe3cjTbMMNYNQHsRS%2FOAQEL%2FMRHDlohxrlmOvc6CeDgcaFYYC%2Bho95pxhKun85Ax3J7rPvTyMq4bA%2FIkR3wTDJBL7qyXeZ3d0%2B52oexjSY54vue%2BbYnb031O6kA8k5Xst3h0dG6EBCuuKW2QdD2cmsfylRTJXvlYZptJatgxnfhY7vwUZvYxG%2F61I0APepxokXJCXqhPpLcn83xlH2u6H2DFquA%3D&u2=rhVbeo1QlsKyPIH%2F&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=6gP5xPTl6OshBEOhdAO8iMMMwiwD%2FHeB54A4s8G%2F0uN7y654MZ1j32N3JRKTmXTjFVGDxDge%2FP8Z%2BhCpLNREZ%2F1HzGfdjj%2BYrud7VVrQ2%2F%2Bs6Ly8ug%3D%3D&u2=tQc7UKu%2Fk6SsbII8&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=3kNpdgHdoLiZ7jK7EG0si6rvD4P%2BC1n%2Br%2BTV9mTP%2BqzFlIXiJEkJRRgEAYG38qrm1mWzAck0sM6lCKdIhVjsammRYvEIubU2LE89Vn5%2Fgue8PNEV8SjcWYO%2Fd7A%3D&u2=6LI26fcv%2FKKI7%2FX3&width=2560"
      ],
      note: "文字盤：黒　ベゼル：黒　コマ数：8",
      revisions: []
    },
    {
      code: "20260303001", brand: "オメガ", brandEn: "Omega", model: "スピードマスター", modelEn: "Speedmaster",
      ref: "311.30.42.30.01.005", serial: "87654321", supplier: "S002", staff: "佐藤 花子",
      purchasePrice: 320000, salePrice: 498000, purchaseDate: "2026-03-03", status: "在庫中",
      material: "MAT-001", movement: "MOV-002", condition: "CON-004", accessories: ["BOX"],
      images: [
        "https://sspark.genspark.ai/cfimages?u1=N2thZtxaem5YnQZDti31UJK5rXBwTM8nFa1JpHYdxIPcPBN6YQz5iLYMRfdoJQRYwA2CTPe6NWUksssOX%2Fh1u4pv%2Fl0trdbNG00VsgAegOY2OmeTi5fuhWkAO0P4rNJeqtpcJb4Vy5%2FtuQJa2uWrqrR5Etg%2BAzcDaS9MIEekU3VjvMD%2FpKG4il5zopQayYQ%3D&u2=52akKsJQKor%2BcWJW&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=PUcS9qimEdQj2%2BeQYbdwiro1TDf1fwM5O3OO7qsOLbxggfoOZ0kU4eqzGCV3Z6MJbCw3XS82%2FwOzNs9J%2F7LeIqXwcOOEIK4qVPTGUCE3g89J8JcWisvY74JAMYKkE4UjoD7uJ48GnoZBin1weaKKCk2Gz9ZPaXYSCqoprLZMnAYq46LfCZiLjuP%2BYAhZUBLRhh0A5L4%2F8WR7urlC2dhl2dKETEB8aea4qRWOfNjI91UUkmR4ITqGk9%2BGXUdNk3CfSTHiLcD6GtMe%2F1A%3D&u2=tzokX0ua%2BWYl5mA4&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=UlQqXMzSTjD9pwW29kImi9m7c4MBgsfwOPCeyNlFfNvdQ01i0DdyNEHTdmi4RaAdUcDNgHGVZcXshuTMLKexYSakraDCZEXUwIq2VEh6B5AxIq3HCgm%2FkkfHyxFzculguflLw%3D%3D&u2=z4OA6PE9QDi1XiTZ&width=2560"
      ],
      note: "文字盤：黒　ベゼル：タキメーター",
      revisions: []
    },
    {
      code: "20260305001", brand: "パテック・フィリップ", brandEn: "Patek Philippe", model: "アクアノート", modelEn: "Aquanaut",
      ref: "5167A-001", serial: "PH112233", supplier: "S005", staff: "山本 太郎",
      purchasePrice: 2800000, purchasePlannedPrice: 2600000, purchaseDate: "2026-03-05", status: "売上済",
      material: "MAT-001", movement: "MOV-001", condition: "CON-003", accessories: ["BOX", "GUARANTEE"],
      images: [], note: "",
      revisions: []
    },
    {
      code: "20260307001", brand: "カルティエ", brandEn: "Cartier", model: "サントス", modelEn: "Santos",
      ref: "WSSA0009", serial: "CT445566", supplier: "S003", staff: "鈴木 一郎",
      purchasePrice: 480000, purchasePlannedPrice: 450000, salePrice: 720000, purchaseDate: "2026-03-07", status: "在庫中",
      material: "MAT-001", movement: "MOV-003", condition: "CON-004", accessories: ["BOX", "GUARANTEE"],
      images: [
        "https://sspark.genspark.ai/cfimages?u1=bIHK3aaCO%2FdOI5DtzcAuCwyh4KebsUCQ7ego4fj4gsgSHyod8AHR3fP9HZj%2BPi1RjHXurVPx9JYbxD9yfS6T1IcQgFwWN85lnowRdmYj6vxdhq4DzpNzAj9L3A%3D%3D&u2=0KiLTYScju95jKog&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=djlGnOoOJOGJBWAN9nAMb4ZDgfR5b6SVCNHAAjSdtmQixBtQCviF16%2F7JeXsdFJV92xmsbpvezRbsxeYLvkqR8qwRFJDcgs1bv09o003JJ6udLftqjyBHSHX7r1f2hvg&u2=L2j3oSCXIQ9BZ7jG&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=wwlL2pAB7mLF0YsVf%2F7yKmQC5tI8vtnJuSCBGkLtVf3Trp2LgSbDE8ULj%2FdkPdIVdtsPU%2FnvCQjnf7izYE3rjAS363l7PZw1m6sTNsS5SnHxRPw28lfa8SIJ3SsDPXdxpGqe9l90CE1dErSgC567Z%2Fy49QXb6mJkHIx1PgSQptVziUGGPVT279Pm3nb675lADdScxBmqWQ85BH5PMwM%3D&u2=U2gy5RDjo5fgW89t&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=bsBtu4%2FM1LgqBVbeVbdkgyLFvnJn%2BoN%2F3A1bn%2FMk5Fy27Xav5khWmeMfpks049rNi0OAP%2BV%2Fr1Vk99KZPiM7QxBSSaXa5Cni9hD9LAnDJ5Cr563yRw%3D%3D&u2=dBh99rKvhkr0%2FJru&width=2560"
      ],
      note: "文字盤：シルバー",
      revisions: []
    },
    {
      code: "20260310001", brand: "ロレックス", brandEn: "Rolex", model: "デイトナ", modelEn: "Daytona",
      ref: "116500LN", serial: "ZX789012", supplier: "S001", staff: "山本 太郎",
      purchasePrice: 1950000, purchasePlannedPrice: 1850000, purchaseDate: "2026-03-10", status: "出荷済",
      material: "MAT-001", movement: "MOV-001", condition: "CON-003", accessories: ["BOX", "GUARANTEE", "BRACELET PARTS"],
      images: [], note: "",
      revisions: []
    },
    {
      code: "20260312001", brand: "IWC", brandEn: "IWC", model: "ポルトギーゼ", modelEn: "Portugieser",
      ref: "IW500705", serial: "IW334455", supplier: "S004", staff: "田中 美香",
      purchasePrice: 560000, salePrice: 840000, purchaseDate: "2026-03-12", status: "在庫中",
      material: "MAT-003", movement: "MOV-001", condition: "CON-004", accessories: ["BOX"],
      images: [
        "https://sspark.genspark.ai/cfimages?u1=MNbxcAbDbmpsfSaruW%2F26IWj9pjYMM7rLPyxWRtuPDNm9jaqXHty%2Fvr3f5UzYLTM%2FaIZR34x%2FD95zWkSEqRHWLXe2JQolFWlNOMdvlW3tSHZJPI0QeBCxvql6pmIKrSTrkP0lUWg&u2=oSViHra1T%2BLPkUxi&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=o%2FSJXvPZwG1uA4we0mPhXTCFL6iadWPvZ8lyLE%2B%2Brlf%2Bww%2BZ1Y2RrDNA7yyDemOltjUKcDGk7%2B5mUFz63IyeyiKsY%2BAW69XhykngIpvML2deCDpgT5ni7gJfrW%2FPzlW05ZyArfo736VSKt4Dok96F7Yhh8Xmj3gYwkF7%2BGd2FmA53FbfZNPXaKscRZdsRQ%3D%3D&u2=bv6xA77DVjrrgNrH&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=wNVpHDyMu9GGKL%2FB31D1dUyqsvB3NAihmuUG3JZlm4IjsZZnnMS%2FdwRmDqRGRgB8MDcom4%2BmBbiS3D92LhpQtQwhFBOkb5P3euDCYivQivoc0DsvFBEJJ5pyCCAbNA8kazOvu6xjVAQyxc%2Bpb5JGKYU%3D&u2=9XmsM9YlFPfPj0L9&width=2560"
      ],
      note: "",
      revisions: []
    },
    {
      code: "20260314001", brand: "グランドセイコー", brandEn: "Grand Seiko", model: "エレガンスコレクション", modelEn: "Elegance Collection",
      ref: "SBGW047", serial: "GS556677", supplier: "S002", staff: "伊藤 健司",
      purchasePrice: 280000, salePrice: 430000, purchaseDate: "2026-03-14", status: "在庫中",
      material: "MAT-005", movement: "MOV-002", condition: "CON-003", accessories: ["BOX", "GUARANTEE"],
      images: [
        "https://sspark.genspark.ai/cfimages?u1=G2W0piFUXOC8euP30%2BfBfj%2FjipK5Me%2BAQW0%2F3g5EtzWaNhRDETdP45N6y7iUPTnff9ZFm0SgZbl6ZiwAJzNIM9HVBvc%2BDNYK3UcX8CZOcfGrnSIO1jFs9TwXKr1n&u2=hChVWqaU4GS%2BhfpE&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=nqte9QQnMUYYF7rXS%2FFqA8MxAjQ3K6ymdfn%2F8W%2F1k8S7e41XAyYFTq62v%2FEbsv%2B9BTrLXXcvbTw5B1J4OH7UHeOxzDaHCuxOTJehabtZfSGOH3fJYGLhZN3W&u2=lA7vi4NZcysYa7YD&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=gfyMsObQHMCMklWh1DrzAUx5NyltSS5Gi7LdTm59ioAMGPpBty28aZG1QlVLBspBbmBkvH07SW8AQsi1pN2i4pTjEZt1qLQUuGRHp7AzdORNRb8EAgaJLk6dlg%3D%3D&u2=33TJYAtXgAnUxFs4&width=2560"
      ],
      note: "",
      revisions: []
    },
    {
      code: "20260315001", brand: "ブライトリング", brandEn: "Breitling", model: "ナビタイマー", modelEn: "Navitimer",
      ref: "AB0121211B1A1", serial: "BR667788", supplier: "S003", staff: "鈴木 一郎",
      purchasePrice: 420000, salePrice: 610000, purchaseDate: "2026-03-15", status: "在庫中",
      material: "MAT-001", movement: "MOV-001", condition: "CON-005", accessories: ["GUARANTEE"],
      images: [
        "https://sspark.genspark.ai/cfimages?u1=AmKHsTQiC35HA%2FFXMCv7H3FQFOF5D4scTRGfZPGeY07hnucMQOAgbLr%2BDnd8%2F8OnZXUBNFKeCdJpdMYIvhB1DTmOADZgTR%2FwlSRJjV5evuLpxf6CUan1xjH0%2BLy%2FsWY7fp9ex18FxfwkWkIkCg%2BDsYNNKWwyqywbJlrbHLE48gL4lCI673%2F0RszV8TkGyGxOtvZqJ6iqmrcN&u2=je4N5jfjagL0CQzI&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=NNWEn9S8baXNlf60LgEM%2F8scB6%2BDEoc4Nb5jlK9Laman3aZ8vUWkSl%2BDgZ8RsRWwIdtQwmm64Rj%2FULR2yZVZ%2B1XstUlHhd14gFfBbp%2FydJUNNKqS2WzABFBBGWFmrZdBr8nIhXngfvycWnDbwrP%2BEEyN%2FMMij%2F4GrcWkalZOa1j1oWrlo37Ffl7EkQ5q2d2oAMRFOtg7BUUmnU8m&u2=MHbq8ol4rRDsjvqT&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=FAifZk%2B3%2BUmKq6m8rF1J%2BSXqW861AY%2FvAnTxb7qs7wc9RVdn923hmH19h%2BScJQ9a7CpVVkj5rJfvZSSY57PN6F5h6IOMkw6nkskGc%2FOMt%2FpXbzM6em7eJGpUBtE%2B%2BFHICke%2Fqse3ZGk1H2ywToRtkpr5gCPkUEDADxlZEGuDjssFcT9awNA%3D&u2=8F7rkWLMSoU1X3vn&width=2560",
        "https://sspark.genspark.ai/cfimages?u1=9NLxAkiG5MH7gSBRneswVTVtuDECmBBA4x%2B%2Bp7un8UGM0Qm5r92SSeIQ3rirxYegDaaj9oM2cBA9SVob3c%2Bx5mdK9EFKg38OnuYV1Ynj3w%3D%3D&u2=aS%2FaNyisnEi7qm9J&width=2560"
      ],
      note: "コマ数：7　リューズ使用感あり",
      revisions: []
    },
    // APR-002 出荷修正で参照される商品
    {
      code: "20260301002", brand: "ロレックス", brandEn: "Rolex", model: "デイトナ（ホワイトゴールド）", modelEn: "Daytona WG",
      ref: "116519LN", serial: "ZX234567", supplier: "S001", staff: "山本 太郎",
      purchasePrice: 1680000, salePrice: 2200000, purchaseDate: "2026-03-01", status: "在庫中",
      material: "MAT-003", movement: "MOV-001", condition: "CON-002", accessories: ["BOX", "CASE", "GUARANTEE"],
      images: [], note: "",
      revisions: []
    },
    // APR-007 仕入修正で参照される商品
    {
      code: "20260310002", brand: "ブライトリング", brandEn: "Breitling", model: "ナビタイマー（ブラック）", modelEn: "Navitimer Black",
      ref: "AB0127211B1A1", serial: "XY001234", supplier: "S002", staff: "佐藤 花子",
      purchasePrice: 480000, salePrice: 720000, purchaseDate: "2026-03-10", status: "在庫中",
      material: "MAT-002", movement: "MOV-001", condition: "CON-003", accessories: ["BOX", "GUARANTEE"],
      images: [], note: "シリアル修正申請中（APR-007）",
      revisions: []
    },
  ],

  // 売上データ
  sales: [
    {
      id: "SL-2026-0001", date: "2026-03-08",
      items: [
        { code: "20260305001", brand: "パテック・フィリップ", model: "アクアノート", salePrice: 3200000,
          returnType: null, returnStatus: null }
      ],
      total: 3200000, buyer: "B001", note: "",
      revisions: []
    },
    {
      id: "SL-2026-0002", date: "2026-03-11",
      items: [
        { code: "20260310001", brand: "ロレックス", model: "デイトナ", salePrice: 2350000,
          returnType: null, returnStatus: null }
      ],
      total: 2350000, buyer: "B002", note: "",
      revisions: [
        { revisedAt: "2026-03-12 09:30", buyerName: "山本 太郎", approverName: "管理者", note: "金額を¥2,350,000に修正（当初¥2,400,000）" }
      ]
    },
    {
      // 返品・持ち帰りサンプル伝票①
      id: "SL-2026-0003", date: "2026-04-01",
      items: [
        {
          code: "20260307001", brand: "カルティエ", model: "サントス",
          salePrice: 720000,
          returnType: "return",       // 返品
          returnStatus: "pending"
        },
        {
          code: "20260314001", brand: "グランドセイコー", model: "エレガンスコレクション",
          salePrice: 430000,
          returnType: null, returnStatus: null   // 通常販売
        }
      ],
      total: 1150000, buyer: "B001", note: "カルティエは返品対応",
      revisions: []
    },
    {
      // 返品・持ち帰りサンプル伝票②（複数対象）
      id: "SL-2026-0004", date: "2026-04-05",
      items: [
        {
          code: "20260312001", brand: "IWC", model: "ポルトギーゼ",
          salePrice: 840000,
          returnType: "takeback",     // 持ち帰り
          returnStatus: "pending"
        },
        {
          code: "20260315001", brand: "ブライトリング", model: "ナビタイマー",
          salePrice: 610000,
          returnType: "return",       // 返品
          returnStatus: "pending"
        }
      ],
      total: 1450000, buyer: "B002", note: "IWC持ち帰り・ブライトリング返品",
      revisions: []
    },
    // APR-006 売上修正で参照（承認済・修正あり）
    {
      id: "SL-2026-0005", date: "2026-03-28",
      items: [
        { code: "20260307001", brand: "カルティエ", model: "サントス", salePrice: 1150000,
          returnType: null, returnStatus: null }
      ],
      total: 1150000, buyer: "B001", note: "端数値引き対応",
      revisions: [
        { revisedAt: "2026-03-29 10:15", buyerName: "佐藤 花子", approverName: "管理者", note: "販売金額を$7,613→$7,419に修正（端数値引き、APR-006承認済）" }
      ]
    },
    // APR-003 売上確定申請中（pendingApproval フラグ付き）
    {
      id: "SL-2026-0008", date: "2026-04-12",
      items: [
        { code: "20260301003", brand: "オメガ", model: "シーマスター", salePrice: 620000,
          returnType: null, returnStatus: null },
        { code: "20260301004", brand: "パネライ", model: "ルミノール", salePrice: 880000,
          returnType: null, returnStatus: null }
      ],
      total: 1500000, buyer: "B002", note: "2点セット割引成約",
      pendingApprovalId: "APR-003",
      status: "承認待ち",
      approvalNote: "2点セットでの割引成約のため、承認をお願いします。",
      approvalBy: "山本 太郎",
      approvalAt: "2026-04-12 14:08",
      approvalChanges: [{ field: "合計金額（USD）", before: "$10,323", after: "$9,677", reason: "セット割引適用" }],
      revisions: []
    },
    // ⑤ サンプル：承認待ち②
    {
      id: "SL-2026-0009", date: "2026-04-15",
      items: [
        { code: "20260301001", brand: "ロレックス", model: "サブマリーナ", salePrice: 1200000,
          returnType: null, returnStatus: null }
      ],
      total: 1200000, buyer: "B003", note: "新規取引先への売上",
      status: "承認待ち",
      approvalNote: "新規取引先への初回売上です。確認をお願いします。",
      approvalBy: "佐藤 花子",
      approvalAt: "2026-04-15 11:20",
      approvalChanges: [],
      revisions: []
    },
    // ⑤ サンプル：差戻し
    {
      id: "SL-2026-0010", date: "2026-04-16",
      items: [
        { code: "20260310002", brand: "ブライトリング", model: "ナビタイマー（ブラック）", salePrice: 750000,
          returnType: null, returnStatus: null }
      ],
      total: 750000, buyer: "B001", note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "売価が市場相場を下回っています。再確認して再申請してください。",
      revisions: []
    },
    // ⑤ サンプル：差戻し②
    {
      id: "SL-2026-0011", date: "2026-04-17",
      items: [
        { code: "20260301002", brand: "ロレックス", model: "デイトナ（WG）", salePrice: 2100000,
          returnType: null, returnStatus: null }
      ],
      total: 2100000, buyer: "B004", note: "",
      status: "差戻し",
      revisionComment: "取引条件の書類が不備です。書類を整えて再申請してください。",
      revisions: []
    },
  ],

  // 出荷データ
  shipments: [
    {
      id: "SH-2026-0001", date: "2026-03-12",
      destination: "B001",
      items: [{ code: "20260310001", brand: "ロレックス", model: "デイトナ", wholesale: 2300000 }],
      total: 2300000, note: "",
      status: "処理済",
      revisions: []
    },
    // APR-002 出荷修正で参照（pendingApproval フラグ付き）
    {
      id: "SH-2026-0003", date: "2026-04-08",
      destination: "B001",
      items: [{ code: "20260301002", brand: "ロレックス", model: "デイトナ（ホワイトゴールド）", wholesale: 2200000 }],
      total: 2200000, note: "値引き交渉により金額修正申請中",
      pendingApprovalId: "APR-002",
      status: "承認待ち",
      approvalNote: "出荷金額を修正しました。ご確認ください。",
      approvalBy: "佐藤 花子",
      approvalAt: "2026-04-09 10:15",
      approvalChanges: [{ field: "合計金額（USD）", before: "$15,226", after: "$14,194", reason: "値引き交渉成立" }],
      revisions: []
    },
    // ⑤ サンプル：承認待ち
    {
      id: "SH-2026-0004", date: "2026-04-11",
      destination: "B002",
      items: [{ code: "20260305001", brand: "ロレックス", model: "サブマリーナ", wholesale: 1150000 }],
      total: 1150000, note: "承認申請中",
      status: "承認待ち",
      approvalNote: "新規取引先への出荷です。確認をお願いします。",
      approvalBy: "鈴木 一郎",
      approvalAt: "2026-04-11 15:30",
      approvalChanges: [],
      revisions: []
    },
    // ⑤ サンプル：差戻し
    {
      id: "SH-2026-0005", date: "2026-04-13",
      destination: "B003",
      items: [{ code: "20260303001", brand: "オメガ", model: "スピードマスター", wholesale: 490000 }],
      total: 490000, note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "出荷先住所が不正確です。確認して再申請してください。",
      revisions: []
    },
    // ⑤ サンプル：差戻し②
    {
      id: "SH-2026-0006", date: "2026-04-14",
      destination: "B004",
      items: [
        { code: "20260307001", brand: "カルティエ", model: "サントス", wholesale: 680000 },
        { code: "20260312001", brand: "IWC", model: "ポルトギーゼ", wholesale: 650000 }
      ],
      total: 1330000, note: "",
      status: "差戻し",
      revisionComment: "2点同時出荷は承認が必要です。個別に申請してください。",
      revisions: []
    },
  ],

  // 委託伝票（API接続時はDBデータで置き換える）
  consignments: [],

  // 購入リクエスト（ゲスト）
  purchaseRequests: [
    {
      id: "PR-001",
      guestId: "G001",
      guestName: "クロノス東京",
      buyerCode: "B004",
      clientCompanyCode: "CLI-001",
      date: "2026-03-23 14:32",
      status: "未対応",   // 未対応 / 対応済 / 保留中
      note: "至急確認お願いします",
      items: [
        { itemCode: "20260301001", itemName: "ロレックス サブマリーナ", salePrice: 1180000, itemStatus: "pending" },
        { itemCode: "20260307001", itemName: "カルティエ サントス", salePrice: 780000, itemStatus: "pending" },
      ]
    },
    {
      id: "PR-002",
      guestId: "G001",
      guestName: "クロノス東京",
      buyerCode: "B004",
      clientCompanyCode: "CLI-001",
      date: "2026-03-23 15:10",
      status: "未対応",
      note: "",
      items: [
        { itemCode: "20260312001", itemName: "IWC ポルトギーゼ", salePrice: 890000, itemStatus: "pending" },
      ]
    },
    {
      id: "PR-003",
      guestId: "G002",
      guestName: "タイムレス商会",
      buyerCode: "B002",
      clientCompanyCode: "CLI-002",
      date: "2026-03-24 09:15",
      status: "対応済",
      note: "急ぎではありません",
      items: [
        { itemCode: "20260303001", itemName: "オメガ スピードマスター", salePrice: 650000, itemStatus: "approved" },
        { itemCode: "20260310001", itemName: "グランドセイコー エレガンス", salePrice: 420000, itemStatus: "rejected" },
      ]
    },
  ],

  // ゲストアカウント
  guestAccounts: [
    { id: "G001", name: "クロノス東京",  password: "guest123", company: "クロノス東京株式会社",  buyerCode: "B004", email: "info@chronos-tokyo.co.jp" },
    { id: "G002", name: "タイムレス商会", password: "guest456", company: "タイムレス商会有限会社", buyerCode: "B002", email: "info@timeless.co.jp" },
    { id: "G003", name: "ウォッチマート", password: "Guest-B001-01", company: "ウォッチマート", buyerCode: "B001", email: "guest.b001@local.invalid", active: true, autoProvisioned: true },
    { id: "G004", name: "ラグジュアリーアイランド", password: "Guest-B003-01", company: "ラグジュアリーアイランド", buyerCode: "B003", email: "guest.b003@local.invalid", active: true, autoProvisioned: true },
  ],

  // 仕入返品伝票（サンプルデータ付き）
  purchaseReturns: [
    // 処理済（完了済み）
    {
      id: "PR-RET-0001",
      date: "2026-03-20",
      supplier: "S001",
      items: [
        { code: "20260301001", brand: "ロレックス", model: "サブマリーナ",
          ref: "116610LN", serial: "ZX123456", purchasePrice: 850000, status: "処理済" }
      ],
      reason: "商品不良",
      note: "ケース側面に深い傷が確認されたため返品対応",
      status: "処理済",
      createdBy: "山本 太郎",
      createdAt: "2026-03-20 10:15",
      invoicePrinted: true,
    },
    // 未処理
    {
      id: "PR-RET-0002",
      date: "2026-04-02",
      supplier: "S003",
      items: [
        { code: "20260307001", brand: "カルティエ", model: "サントス",
          ref: "WSSA0009", serial: "CT445566", purchasePrice: 480000, status: "未処理" },
        { code: "20260315001", brand: "ブライトリング", model: "ナビタイマー",
          ref: "AB0121211B1A1", serial: "BR667788", purchasePrice: 420000, status: "未処理" },
      ],
      reason: "注文違い",
      note: "発注内容と相違。2点まとめて返品",
      status: "未処理",
      createdBy: "鈴木 一郎",
      createdAt: "2026-04-02 14:30",
      invoicePrinted: false,
    },
    // 承認待ち①
    {
      id: "PR-RET-0003",
      date: "2026-04-10",
      supplier: "S002",
      items: [
        { code: "20260410001", brand: "パテック・フィリップ", model: "アクアノート",
          ref: "5167A-001", serial: "PP556677", purchasePrice: 2600000, status: "未処理" }
      ],
      reason: "品質不適合",
      note: "ケースの仕上げに問題あり。仕入先に返送申請中",
      status: "承認待ち",
      approvalNote: "高額商品のため管理者承認をお願いします。",
      approvalBy: "佐藤 花子",
      approvalAt: "2026-04-10 15:00",
      approvalChanges: [],
      createdBy: "佐藤 花子",
      createdAt: "2026-04-10 15:00",
      invoicePrinted: false,
    },
    // 承認待ち②
    {
      id: "PR-RET-0004",
      date: "2026-04-14",
      supplier: "S004",
      items: [
        { code: "20260415001", brand: "ロレックス", model: "デイトジャスト",
          ref: "126300", serial: "RX445566", purchasePrice: 1350000, status: "未処理" }
      ],
      reason: "説明と相違",
      note: "コンディションが説明より劣化していた。返品対応で合意",
      status: "承認待ち",
      approvalNote: "返品合意済。承認をお願いします。",
      approvalBy: "田中 美香",
      approvalAt: "2026-04-14 10:30",
      approvalChanges: [],
      createdBy: "田中 美香",
      createdAt: "2026-04-14 10:30",
      invoicePrinted: false,
    },
    // 承認待ち③
    {
      id: "PR-RET-0005",
      date: "2026-04-16",
      supplier: "S005",
      items: [
        { code: "20260416001", brand: "オメガ", model: "アクアテラ",
          ref: "220.10.38.20", serial: "", purchasePrice: 280000, status: "未処理" }
      ],
      reason: "シリアル番号不一致",
      note: "書類と現物のシリアル番号が一致しない",
      status: "承認待ち",
      approvalNote: "真贋確認のため返品申請。管理者確認をお願いします。",
      approvalBy: "伊藤 健司",
      approvalAt: "2026-04-16 14:00",
      approvalChanges: [],
      createdBy: "伊藤 健司",
      createdAt: "2026-04-16 14:00",
      invoicePrinted: false,
    },
    // 差戻し①
    {
      id: "PR-RET-0006",
      date: "2026-04-15",
      supplier: "S002",
      items: [
        { code: "20260417001", brand: "パテック・フィリップ", model: "カラトラバ",
          ref: "5396G-001", serial: "PP667788", purchasePrice: 3200000, status: "未処理" }
      ],
      reason: "商品説明相違",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "返品理由の証拠書類（写真）を添付して再申請してください。",
      createdBy: "山本 太郎",
      createdAt: "2026-04-15 11:00",
      invoicePrinted: false,
    },
    // 差戻し②
    {
      id: "PR-RET-0007",
      date: "2026-04-17",
      supplier: "S003",
      items: [
        { code: "20260414001", brand: "IWC", model: "ポルトギーゼ",
          ref: "IW500704", serial: "IW334455", purchasePrice: 450000, status: "未処理" }
      ],
      reason: "梱包不良",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "仕入先との往復書面を添付して再申請してください。",
      createdBy: "鈴木 一郎",
      createdAt: "2026-04-17 09:00",
      invoicePrinted: false,
    },
    // 差戻し③
    {
      id: "PR-RET-0008",
      date: "2026-04-17",
      supplier: "S001",
      items: [
        { code: "20260414002", brand: "ブライトリング", model: "ナビタイマー",
          ref: "AB0121", serial: "BR778899", purchasePrice: 390000, status: "未処理" }
      ],
      reason: "不良品",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "不良内容を詳細に記載して再申請してください。",
      createdBy: "佐藤 花子",
      createdAt: "2026-04-17 13:30",
      invoicePrinted: false,
    },
    // 承認済①（配送番号入力可能なサンプル）
    {
      id: "PR-RET-0009",
      date: "2026-04-10",
      supplier: "S001",
      items: [
        { code: "20260301001", brand: "ロレックス", model: "サブマリーナ",
          ref: "116610LN", serial: "ZX223300", purchasePrice: 850000,
          status: "処理済", trackingNo: "JMOV-0012345678JP" },
        { code: "20260303001", brand: "オメガ", model: "スピードマスター",
          ref: "310.30.42.50.01.001", serial: "OM112233", purchasePrice: 320000,
          status: "処理済", trackingNo: "" }
      ],
      reason: "商品不良",
      note: "処理済サンプル（配送番号入力可・1点は入力済み）",
      status: "処理済",
      createdBy: "山本 太郎",
      createdAt: "2026-04-10 09:00",
      invoicePrinted: true,
    },
    // 承認済②（配送番号入力済み）
    {
      id: "PR-RET-0010",
      date: "2026-04-12",
      supplier: "S002",
      items: [
        { code: "20260305001", brand: "パテック・フィリップ", model: "カラトラバ",
          ref: "5196G", serial: "PP334455", purchasePrice: 2800000,
          status: "処理済", trackingNo: "SF123456789JP" }
      ],
      reason: "説明と相違",
      note: "処理済サンプル（配送番号入力済み）",
      status: "処理済",
      createdBy: "佐藤 花子",
      createdAt: "2026-04-12 11:00",
      invoicePrinted: true,
    },
  ],

  // 売上返品伝票
  salesReturns: [
    // 未処理
    {
      id: "SR-2026-0001",
      date: "2026-04-05",
      slipId: "SL-2026-0003",
      buyer: "B001",
      items: [
        { code: "20260307001", brand: "カルティエ", model: "サントス",
          ref: "WSSA0009", serial: "CT445566", salePrice: 720000 }
      ],
      total: 720000,
      reason: "商品不良",
      note: "開封後に裏蓋に傷が見つかった",
      status: "未処理",
      createdBy: "山本 太郎",
      createdAt: "2026-04-05 11:30",
      invoicePrinted: false,
    },
    // 承認待ち①
    {
      id: "SR-2026-0002",
      date: "2026-04-12",
      slipId: "SL-2026-0004",
      buyer: "B002",
      items: [
        { code: "20260312001", brand: "IWC", model: "ポルトギーゼ",
          ref: "IW500704", serial: "IW334455", salePrice: 840000 },
        { code: "20260315001", brand: "ブライトリング", model: "ナビタイマー",
          ref: "AB0121211B1A1", serial: "BR667788", salePrice: 610000 }
      ],
      total: 1450000,
      reason: "注文違い",
      note: "販売先の手配ミスにより2点返品",
      status: "承認待ち",
      approvalNote: "返品承認申請中。管理者の確認をお願いします。",
      approvalBy: "佐藤 花子",
      approvalAt: "2026-04-12 16:45",
      createdBy: "佐藤 花子",
      createdAt: "2026-04-12 16:45",
      invoicePrinted: false,
    },
    // 承認待ち②
    {
      id: "SR-2026-0003",
      date: "2026-04-15",
      slipId: "SL-2026-0001",
      buyer: "B001",
      items: [
        { code: "20260305001", brand: "パテック・フィリップ", model: "アクアノート",
          ref: "5167A-001", serial: "PH112233", salePrice: 3200000 }
      ],
      total: 3200000,
      reason: "品質不適合",
      note: "販売後にケースの傷が発覚。販売先より返品要求",
      status: "承認待ち",
      approvalNote: "高額品のため管理者承認が必要です。",
      approvalBy: "山本 太郎",
      approvalAt: "2026-04-15 10:00",
      createdBy: "山本 太郎",
      createdAt: "2026-04-15 10:00",
      invoicePrinted: false,
    },
    // 承認待ち③
    {
      id: "SR-2026-0004",
      date: "2026-04-16",
      slipId: "SL-2026-0002",
      buyer: "B002",
      items: [
        { code: "20260310001", brand: "ロレックス", model: "デイトナ",
          ref: "116500LN", serial: "ZX789012", salePrice: 2350000 }
      ],
      total: 2350000,
      reason: "気が変わった",
      note: "販売先都合の返品。交渉中",
      status: "承認待ち",
      approvalNote: "返品条件を確認中。承認をお願いします。",
      approvalBy: "鈴木 一郎",
      approvalAt: "2026-04-16 09:30",
      createdBy: "鈴木 一郎",
      createdAt: "2026-04-16 09:30",
      invoicePrinted: false,
    },
    // 差戻し①
    {
      id: "SR-2026-0005",
      date: "2026-04-16",
      slipId: "SL-2026-0005",
      buyer: "B001",
      items: [
        { code: "20260307001", brand: "カルティエ", model: "サントス",
          ref: "WSSA0009", serial: "CT445566", salePrice: 1150000 }
      ],
      total: 1150000,
      reason: "商品説明相違",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "返品理由の詳細と証拠写真を添付して再申請してください。",
      createdBy: "佐藤 花子",
      createdAt: "2026-04-16 14:00",
      invoicePrinted: false,
    },
    // 差戻し②
    {
      id: "SR-2026-0006",
      date: "2026-04-17",
      slipId: "SL-2026-0008",
      buyer: "B002",
      items: [
        { code: "20260301003", brand: "オメガ", model: "シーマスター",
          ref: "", serial: "", salePrice: 620000 }
      ],
      total: 620000,
      reason: "動作不良",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "修理見積書を添付の上、再申請してください。",
      createdBy: "田中 美香",
      createdAt: "2026-04-17 11:00",
      invoicePrinted: false,
    },
    // 差戻し③
    {
      id: "SR-2026-0007",
      date: "2026-04-17",
      slipId: "SL-2026-0009",
      buyer: "B003",
      items: [
        { code: "20260301001", brand: "ロレックス", model: "サブマリーナ",
          ref: "116610LN", serial: "ZX123456", salePrice: 1200000 }
      ],
      total: 1200000,
      reason: "注文違い",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "返品可否は取引条件書に基づきます。条件書を再確認して再申請してください。",
      createdBy: "伊藤 健司",
      createdAt: "2026-04-17 15:30",
      invoicePrinted: false,
    },
    // 処理済
    {
      id: "SR-2026-0008",
      date: "2026-03-15",
      slipId: "SL-2026-0001",
      buyer: "B001",
      items: [
        { code: "20260305001", brand: "パテック・フィリップ", model: "アクアノート",
          ref: "5167A-001", serial: "PH112233", salePrice: 3200000 }
      ],
      total: 3200000,
      reason: "商品不良",
      note: "早期返品。処理完了",
      status: "処理済",
      createdBy: "管理者",
      createdAt: "2026-03-15 10:00",
      invoicePrinted: true,
    },
  ],

  // ===== 外貨レート設定（円換算レート手入力）=====
  fxRateHistory: [],
  fxRates: [
    {
      code:     "USD",
      name:     "USドル",
      symbol:   "$",
      flag:     "🇺🇸",
      rate:     155.00,          // 1 USD = ? 円
      updatedAt: "2026-04-13 09:00",
      updatedBy: "管理者",
    },
    {
      code:     "EUR",
      name:     "ユーロ",
      symbol:   "€",
      flag:     "🇪🇺",
      rate:     165.00,          // 1 EUR = ? 円
      updatedAt: "2026-04-13 09:00",
      updatedBy: "管理者",
    },
    {
      code:     "HKD",
      name:     "香港ドル",
      symbol:   "HK$",
      flag:     "🇭🇰",
      rate:     19.80,           // 1 HKD = ? 円
      updatedAt: "2026-04-13 09:00",
      updatedBy: "管理者",
    },
    {
      code:     "CHF",
      name:     "スイスフラン",
      symbol:   "Fr",
      flag:     "🇨🇭",
      rate:     172.00,          // 1 CHF = ? 円
      updatedAt: "2026-04-15 09:00",
      updatedBy: "管理者",
    },
  ],

  // ===== ユーザーアカウント（管理者・作業者） =====
  users: [
    { id: "U001", role: "admin",  name: "管理者",     loginId: "admin",  email: "admin@watch-premium.co.jp",  password: "admin123",  avatar: "管", approvalCode: "482910", approvalCodeUpdatedAt: "2026-04-01" },
    { id: "U002", role: "buyer",  name: "山本 太郎",  loginId: "buyer1", email: "yamamoto@watch-premium.co.jp", password: "buyer123",  avatar: "山" },
    { id: "U003", role: "buyer",  name: "佐藤 花子",  loginId: "buyer2", email: "sato@watch-premium.co.jp",    password: "buyer456",  avatar: "佐" },
  ],

  // ===== 承認リクエスト =====
  approvalRequests: [
    // ── 仕入伝票 修正申請（保留中）──
    {
      id: "APR-001",
      buyerId: "U002",
      buyerName: "山本 太郎",
      type: "purchase_edit",
      typeLabel: "仕入伝票 修正",
      detail: {
        slipId: "20260301001",
        brand: "ロレックス", model: "サブマリーナ",
        ref: "116610LN", serial: "ZX123456",
        supplier: "S001",
        purchaseDate: "2026-03-01",
        before: { purchasePrice: 820000, condition: "CON-003" },
        after:  { purchasePrice: 850000, condition: "CON-002" },
        reason: "仕入価格の入力ミスを修正。コンディション評価を再確認の上、Sランクへ変更。",
      },
      status: "pending",
      note: "担当者確認済みです。ご承認をお願いします。",
      revisionComment: "",
      otp: null, otpExpiry: null, otpUsed: false, otpAttempts: 0,
      createdAt: "2026-04-10 09:32",
    },
    // ── 出荷伝票 修正申請（保留中）──
    {
      id: "APR-002",
      buyerId: "U003",
      buyerName: "佐藤 花子",
      type: "shipping_edit",
      typeLabel: "出荷伝票 修正",
      detail: {
        slipId: "SH-2026-0003",
        destination: "B001",
        shipDate: "2026-04-08",
        before: { total: 2360000, items: ["20260301002"] },
        after:  { total: 2200000, items: ["20260301002"], note: "値引き交渉により単価修正" },
        reason: "販売先との合意により出荷金額を修正しました。",
      },
      status: "pending",
      note: "至急対応お願いします。",
      revisionComment: "",
      otp: null, otpExpiry: null, otpUsed: false, otpAttempts: 0,
      createdAt: "2026-04-11 11:15",
    },
    // ── 売上伝票 確定申請（保留中）──
    {
      id: "APR-003",
      buyerId: "U002",
      buyerName: "山本 太郎",
      type: "sales",
      typeLabel: "売上伝票 確定",
      detail: {
        slipId: "SL-2026-0008",
        buyer: "B002",
        saleDate: "2026-04-12",
        items: [
          { code: "20260301003", brand: "オメガ", model: "シーマスター", salePrice: 620000 },
          { code: "20260301004", brand: "パネライ", model: "ルミノール",  salePrice: 880000 },
        ],
        total: 1500000,
        note: "2点セットでのご成約。割引条件あり。",
      },
      status: "pending",
      note: "2点セットでの割引成約のため、承認をお願いします。",
      revisionComment: "",
      otp: null, otpExpiry: null, otpUsed: false, otpAttempts: 0,
      createdAt: "2026-04-12 14:08",
    },
    // ── マスタ登録 申請（差戻し）──
    {
      id: "APR-004",
      buyerId: "U003",
      buyerName: "佐藤 花子",
      type: "master",
      typeLabel: "マスタ登録",
      detail: {
        masterType: "supplier",
        masterLabel: "仕入先",
        action: "新規追加",
        data: {
          code: "S005",
          name: "リユース東京株式会社",
          contact: "03-1234-5678",
          address: "東京都渋谷区○○1-2-3",
          note: "新規取引先。初回取引条件確認済み。",
        },
      },
      status: "revision",
      note: "新規仕入先の追加申請です。",
      revisionComment: "取引条件書の添付が必要です。書類を整えた上で再申請してください。",
      revisionHistory: [
        { comment: "取引条件書の添付が必要です。書類を整えた上で再申請してください。", revisedAt: "2026-04-10 09:30", revisedBy: "管理者" }
      ],
      otp: null, otpExpiry: null, otpUsed: false, otpAttempts: 0,
      createdAt: "2026-04-09 16:45",
    },
    // ── 実績管理 修正申請（承認済）──
    {
      id: "APR-005",
      buyerId: "U002",
      buyerName: "山本 太郎",
      type: "performance",
      typeLabel: "実績管理 修正",
      detail: {
        targetMonth: "2026年3月",
        targetField: "売上実績",
        before: { amount: 5200000, count: 8 },
        after:  { amount: 5550000, count: 9 },
        reason: "3月末日成約分の売上伝票1件（SL-2026-0007）が未計上だったため追加修正。",
      },
      status: "approved",
      note: "3月分の締め後修正です。",
      revisionComment: "",
      otp: "391047",
      otpExpiry: null,
      otpUsed: true,
      otpAttempts: 1,
      createdAt: "2026-04-05 10:00",
    },
    // ── 売上伝票 修正申請（承認済）──
    {
      id: "APR-006",
      buyerId: "U003",
      buyerName: "佐藤 花子",
      type: "sales_edit",
      typeLabel: "売上伝票 修正",
      detail: {
        slipId: "SL-2026-0005",
        buyer: "B001",
        saleDate: "2026-03-28",
        before: { total: 1180000, note: "" },
        after:  { total: 1150000, note: "端数値引き対応" },
        reason: "販売先要望により端数の値引きを実施。担当者確認済み。",
      },
      status: "approved",
      note: "値引き交渉成立のため修正をお願いします。",
      revisionComment: "",
      otp: "827463",
      otpExpiry: null,
      otpUsed: true,
      otpAttempts: 1,
      createdAt: "2026-03-29 09:20",
    },
    // ── 仕入伝票 修正申請（保留中）──
    {
      id: "APR-007",
      buyerId: "U003",
      buyerName: "佐藤 花子",
      type: "purchase_edit",
      typeLabel: "仕入伝票 修正",
      detail: {
        slipId: "20260310002",
        brand: "ブライトリング", model: "ナビタイマー",
        ref: "AB0127211B1A1",
        supplier: "S002",
        purchaseDate: "2026-03-10",
        before: { purchasePrice: 480000, serial: "XX999999" },
        after:  { purchasePrice: 480000, serial: "XY001234" },
        reason: "シリアル番号の転記ミスを修正。現物確認済み。",
      },
      status: "pending",
      note: "シリアル番号の誤記修正です。確認の上ご承認ください。",
      revisionComment: "",
      otp: null, otpExpiry: null, otpUsed: false, otpAttempts: 0,
      createdAt: "2026-04-14 08:55",
    },
    // ── マスタ登録 申請（保留中）──
    {
      id: "APR-008",
      buyerId: "U002",
      buyerName: "山本 太郎",
      type: "master",
      typeLabel: "マスタ登録",
      detail: {
        masterType: "buyer",
        masterLabel: "販売先（取引先）",
        action: "情報更新",
        data: {
          code: "B002",
          name: "大阪ウォッチマーケット",
          contact: "06-9876-5432",
          address: "大阪府大阪市北区○○2-5-10",
          note: "担当者変更。連絡先・住所を最新情報へ更新。",
        },
      },
      status: "pending",
      note: "取引先情報の更新申請です。",
      revisionComment: "",
      otp: null, otpExpiry: null, otpUsed: false, otpAttempts: 0,
      createdAt: "2026-04-15 10:20",
    },
  ],

  // ===== 通知 =====
  notifications: [
    {
      id: "NTF-001",
      toUserId: "U001",
      fromUserId: "U002",
      fromName: "山本 太郎",
      type: "approval_request",
      title: "承認リクエスト：仕入伝票 修正",
      body: "山本 太郎 さんが仕入伝票（20260301001）の修正承認を申請しました。内容を確認してください。",
      relatedId: "APR-001",
      read: false,
      createdAt: "2026-04-10 09:32",
    },
    {
      id: "NTF-002",
      toUserId: "U001",
      fromUserId: "U003",
      fromName: "佐藤 花子",
      type: "approval_request",
      title: "承認リクエスト：出荷伝票 修正",
      body: "佐藤 花子 さんが出荷伝票（SH-2026-0003）の修正承認を申請しました。至急対応をお願いします。",
      relatedId: "APR-002",
      read: false,
      createdAt: "2026-04-11 11:15",
    },
    {
      id: "NTF-003",
      toUserId: "U001",
      fromUserId: "U002",
      fromName: "山本 太郎",
      type: "approval_request",
      title: "承認リクエスト：売上伝票 確定",
      body: "山本 太郎 さんが売上伝票（SL-2026-0008）の確定承認を申請しました。2点セット成約・割引条件あり。",
      relatedId: "APR-003",
      read: false,
      createdAt: "2026-04-12 14:08",
    },
    {
      id: "NTF-004",
      toUserId: "U003",
      fromUserId: "U001",
      fromName: "管理者",
      type: "revision",
      title: "差戻し：マスタ登録",
      body: "マスタ登録（仕入先 新規追加）の申請が差し戻されました。取引条件書の添付が必要です。書類を整えた上で再申請してください。",
      relatedId: "APR-004",
      read: false,
      createdAt: "2026-04-09 17:00",
    },
    {
      id: "NTF-005",
      toUserId: "U002",
      fromUserId: "U001",
      fromName: "管理者",
      type: "approved",
      title: "承認完了：実績管理 修正",
      body: "実績管理（2026年3月 売上実績）の修正申請が承認されました。承認コード: 391047",
      relatedId: "APR-005",
      read: true,
      createdAt: "2026-04-05 10:30",
    },
    {
      id: "NTF-006",
      toUserId: "U001",
      fromUserId: "U003",
      fromName: "佐藤 花子",
      type: "approval_request",
      title: "承認リクエスト：仕入伝票 修正",
      body: "佐藤 花子 さんが仕入伝票（20260310002）のシリアル番号修正承認を申請しました。",
      relatedId: "APR-007",
      read: false,
      createdAt: "2026-04-14 08:55",
    },
    {
      id: "NTF-007",
      toUserId: "U001",
      fromUserId: "U002",
      fromName: "山本 太郎",
      type: "approval_request",
      title: "承認リクエスト：マスタ登録",
      body: "山本 太郎 さんが販売先情報（大阪ウォッチマーケット）の更新承認を申請しました。",
      relatedId: "APR-008",
      read: false,
      createdAt: "2026-04-15 10:20",
    },
  ],

  // ===== BOXグループ (番号 1〜10 固定) =====
  // boxes[n-1] が BOX番号 n に対応
  // publicTo: 公開先コードの配列（空配列 = 全社非公開）
  boxes: [
    { no: 1,  name: "ロレックス特集",   publicTo: ["B001", "B002"], createdAt: "2026-03-01" },
    { no: 2,  name: "高額品セレクト",   publicTo: [],               createdAt: "2026-03-05" },
    { no: 3,  name: "春の新入荷",       publicTo: ["B001"],         createdAt: "2026-03-20" },
    { no: 4,  name: "",                 publicTo: [],               createdAt: "" },
    { no: 5,  name: "",                 publicTo: [],               createdAt: "" },
    { no: 6,  name: "",                 publicTo: [],               createdAt: "" },
    { no: 7,  name: "",                 publicTo: [],               createdAt: "" },
    { no: 8,  name: "",                 publicTo: [],               createdAt: "" },
    { no: 9,  name: "",                 publicTo: [],               createdAt: "" },
    { no: 10, name: "",                 publicTo: [],               createdAt: "" },
  ],

  // =====================================================
  // ゲスト公開スナップショット
  // 「公開情報を一括更新」ボタン押下時にのみ更新される。
  // ゲスト画面はこのスナップショットを参照するため、
  // チェック変更が即時反映されない。
  // 構造: {
  //   updatedAt: "YYYY/MM/DD HH:mm" | null,
  //   boxes: [
  //     {
  //       no: Number,
  //       name: String,
  //       publicTo: [buyerCode, ...],   // 公開先企業コード
  //       items: [{ code, brand, model, ref, salePrice, condition, status, boxNo }]
  //     }, ...
  //   ]
  // }
  publishedSnapshot: {
    updatedAt: "2026-04-19 10:00",
    boxes: [
      {
        no: 1,
        name: "ロレックス特集",
        publicTo: ["B001", "B002", "B003", "B004"],
        items: [
          {
            code: "20260301001", brand: "ロレックス", brandEn: "Rolex",
            model: "サブマリーナ", modelEn: "Submariner",
            ref: "116610LN", serial: "ZX123456",
            salePrice: 1180000, condition: "CON-003", status: "在庫中", boxNo: 1,
            material: "MAT-001", movement: "MOV-001", accessories: ["BOX", "GUARANTEE"],
            images: [
              "https://sspark.genspark.ai/cfimages?u1=DgG%2FT%2FRiDi119SYIwu0kNOmGnlMd2p6TvXoiRTIyu5aLuI7OGoVdX7Vm5NOBuuyHc02AW9wpJ3XI7O19LikN1pjtVJF5dwqWH7CvGI0aacVG8RFyulGV1ez9EjAOUNqQdE0Pg2Qxaww3NmYj38YAEL0DGbqW6NB29GB5t7hb67g%3D&u2=%2FEq%2BGAMH3r7lDW2h&width=2560",
              "https://sspark.genspark.ai/cfimages?u1=xe3cjTbMMNYNQHsRS%2FOAQEL%2FMRHDlohxrlmOvc6CeDgcaFYYC%2Bho95pxhKun85Ax3J7rPvTyMq4bA%2FIkR3wTDJBL7qyXeZ3d0%2B52oexjSY54vue%2BbYnb031O6kA8k5Xst3h0dG6EBCuuKW2QdD2cmsfylRTJXvlYZptJatgxnfhY7vwUZvYxG%2F61I0APepxokXJCXqhPpLcn83xlH2u6H2DFquA%3D&u2=rhVbeo1QlsKyPIH%2F&width=2560"
            ],
            note: "文字盤：黒　ベゼル：黒　コマ数：8"
          },
          {
            code: "20260301002", brand: "ロレックス", brandEn: "Rolex",
            model: "デイトナ（ホワイトゴールド）", modelEn: "Daytona WG",
            ref: "116519LN", serial: "ZX234567",
            salePrice: 2200000, condition: "CON-002", status: "在庫中", boxNo: 1,
            material: "MAT-003", movement: "MOV-001", accessories: ["BOX", "CASE", "GUARANTEE"],
            images: [],
            note: ""
          }
        ]
      },
      {
        no: 3,
        name: "春の新入荷",
        publicTo: ["B001", "B002", "B003", "B004"],
        items: [
          {
            code: "20260303001", brand: "オメガ", brandEn: "Omega",
            model: "スピードマスター", modelEn: "Speedmaster",
            ref: "311.30.42.30.01.005", serial: "87654321",
            salePrice: 498000, condition: "CON-004", status: "在庫中", boxNo: 3,
            material: "MAT-001", movement: "MOV-002", accessories: ["BOX"],
            images: [
              "https://sspark.genspark.ai/cfimages?u1=N2thZtxaem5YnQZDti31UJK5rXBwTM8nFa1JpHYdxIPcPBN6YQz5iLYMRfdoJQRYwA2CTPe6NWUksssOX%2Fh1u4pv%2Fl0trdbNG00VsgAegOY2OmeTi5fuhWkAO0P4rNJeqtpcJb4Vy5%2FtuQJa2uWrqrR5Etg%2BAzcDaS9MIEekU3VjvMD%2FpKG4il5zopQayYQ%3D&u2=52akKsJQKor%2BcWJW&width=2560",
              "https://sspark.genspark.ai/cfimages?u1=PUcS9qimEdQj2%2BeQYbdwiro1TDf1fwM5O3OO7qsOLbxggfoOZ0kU4eqzGCV3Z6MJbCw3XS82%2FwOzNs9J%2F7LeIqXwcOOEIK4qVPTGUCE3g89J8JcWisvY74JAMYKkE4UjoD7uJ48GnoZBin1weaKKCk2Gz9ZPaXYSCqoprLZMnAYq46LfCZiLjuP%2BYAhZUBLRhh0A5L4%2F8WR7urlC2dhl2dKETEB8aea4qRWOfNjI91UUkmR4ITqGk9%2BGXUdNk3CfSTHiLcD6GtMe%2F1A%3D&u2=tzokX0ua%2BWYl5mA4&width=2560"
            ],
            note: "文字盤：黒　ベゼル：タキメーター"
          },
          {
            code: "20260307001", brand: "カルティエ", brandEn: "Cartier",
            model: "サントス", modelEn: "Santos",
            ref: "WSSA0009", serial: "CT445566",
            salePrice: 720000, condition: "CON-004", status: "在庫中", boxNo: 3,
            material: "MAT-001", movement: "MOV-003", accessories: ["BOX", "GUARANTEE"],
            images: [
              "https://sspark.genspark.ai/cfimages?u1=bIHK3aaCO%2FdOI5DtzcAuCwyh4KebsUCQ7ego4fj4gsgSHyod8AHR3fP9HZj%2BPi1RjHXurVPx9JYbxD9yfS6T1IcQgFwWN85lnowRdmYj6vxdhq4DzpNzAj9L3A%3D%3D&u2=0KiLTYScju95jKog&width=2560",
              "https://sspark.genspark.ai/cfimages?u1=djlGnOoOJOGJBWAN9nAMb4ZDgfR5b6SVCNHAAjSdtmQixBtQCviF16%2F7JeXsdFJV92xmsbpvezRbsxeYLvkqR8qwRFJDcgs1bv09o003JJ6udLftqjyBHSHX7r1f2hvg&u2=L2j3oSCXIQ9BZ7jG&width=2560"
            ],
            note: "文字盤：シルバー"
          },
          {
            code: "20260312001", brand: "IWC", brandEn: "IWC",
            model: "ポルトギーゼ", modelEn: "Portugieser",
            ref: "IW500705", serial: "IW334455",
            salePrice: 840000, condition: "CON-004", status: "在庫中", boxNo: 3,
            material: "MAT-003", movement: "MOV-001", accessories: ["BOX"],
            images: [
              "https://sspark.genspark.ai/cfimages?u1=MNbxcAbDbmpsfSaruW%2F26IWj9pjYMM7rLPyxWRtuPDNm9jaqXHty%2Fvr3f5UzYLTM%2FaIZR34x%2FD95zWkSEqRHWLXe2JQolFWlNOMdvlW3tSHZJPI0QeBCxvql6pmIKrSTrkP0lUWg&u2=oSViHra1T%2BLPkUxi&width=2560",
              "https://sspark.genspark.ai/cfimages?u1=o%2FSJXvPZwG1uA4we0mPhXTCFL6iadWPvZ8lyLE%2B%2Brlf%2Bww%2BZ1Y2RrDNA7yyDemOltjUKcDGk7%2B5mUFz63IyeyiKsY%2BAW69XhykngIpvML2deCDpgT5ni7gJfrW%2FPzlW05ZyArfo736VSKt4Dok96F7Yhh8Xmj3gYwkF7%2BGd2FmA53FbfZNPXaKscRZdsRQ%3D%3D&u2=bv6xA77DVjrrgNrH&width=2560"
            ],
            note: ""
          },
          {
            code: "20260314001", brand: "グランドセイコー", brandEn: "Grand Seiko",
            model: "エレガンスコレクション", modelEn: "Elegance Collection",
            ref: "SBGW047", serial: "GS556677",
            salePrice: 430000, condition: "CON-003", status: "在庫中", boxNo: 3,
            material: "MAT-005", movement: "MOV-002", accessories: ["BOX", "GUARANTEE"],
            images: [
              "https://sspark.genspark.ai/cfimages?u1=G2W0piFUXOC8euP30%2BfBfj%2FjipK5Me%2BAQW0%2F3g5EtzWaNhRDETdP45N6y7iUPTnff9ZFm0SgZbl6ZiwAJzNIM9HVBvc%2BDNYK3UcX8CZOcfGrnSIO1jFs9TwXKr1n&u2=hChVWqaU4GS%2BhfpE&width=2560",
              "https://sspark.genspark.ai/cfimages?u1=nqte9QQnMUYYF7rXS%2FFqA8MxAjQ3K6ymdfn%2F8W%2F1k8S7e41XAyYFTq62v%2FEbsv%2B9BTrLXXcvbTw5B1J4OH7UHeOxzDaHCuxOTJehabtZfSGOH3fJYGLhZN3W&u2=lA7vi4NZcysYa7YD&width=2560"
            ],
            note: ""
          },
          {
            code: "20260315001", brand: "ブライトリング", brandEn: "Breitling",
            model: "ナビタイマー", modelEn: "Navitimer",
            ref: "AB0121211B1A1", serial: "BR667788",
            salePrice: 610000, condition: "CON-005", status: "在庫中", boxNo: 3,
            material: "MAT-001", movement: "MOV-001", accessories: ["GUARANTEE"],
            images: [
              "https://sspark.genspark.ai/cfimages?u1=AmKHsTQiC35HA%2FFXMCv7H3FQFOF5D4scTRGfZPGeY07hnucMQOAgbLr%2BDnd8%2F8OnZXUBNFKeCdJpdMYIvhB1DTmOADZgTR%2FwlSRJjV5evuLpxf6CUan1xjH0%2BLy%2FsWY7fp9ex18FxfwkWkIkCg%2BDsYNNKWwyqywbJlrbHLE48gL4lCI673%2F0RszV8TkGyGxOtvZqJ6iqmrcN&u2=je4N5jfjagL0CQzI&width=2560",
              "https://sspark.genspark.ai/cfimages?u1=NNWEn9S8baXNlf60LgEM%2F8scB6%2BDEoc4Nb5jlK9Laman3aZ8vUWkSl%2BDgZ8RsRWwIdtQwmm64Rj%2FULR2yZVZ%2B1XstUlHhd14gFfBbp%2FydJUNNKqS2WzABFBBGWFmrZdBr8nIhXngfvycWnDbwrP%2BEEyN%2FMMij%2F4GrcWkalZOa1j1oWrlo37Ffl7EkQ5q2d2oAMRFOtg7BUUmnU8m&u2=MHbq8ol4rRDsjvqT&width=2560"
            ],
            note: "コマ数：7　リューズ使用感あり"
          },
          {
            code: "20260310002", brand: "ブライトリング", brandEn: "Breitling",
            model: "ナビタイマー（ブラック）", modelEn: "Navitimer Black",
            ref: "AB0127211B1A1", serial: "XY001234",
            salePrice: 720000, condition: "CON-003", status: "在庫中", boxNo: 3,
            material: "MAT-002", movement: "MOV-001", accessories: ["BOX", "GUARANTEE"],
            images: [],
            note: ""
          }
        ]
      }
    ],
  },

  // 日付ごとの通し番号管理（キー: "YYYYMMDD", 値: 次の番号）
  // サンプルデータの仕入日に合わせて初期値を設定
  itemNumberByDate: {
    "20260301": 2,  // INV: ロレックス サブマリーナ
    "20260303": 2,  // INV: オメガ スピードマスター
    "20260305": 2,  // INV: パテック・フィリップ アクアノート
    "20260307": 2,  // INV: カルティエ サントス
    "20260310": 2,  // INV: ロレックス デイトナ
    "20260312": 2,  // INV: IWC ポルトギーゼ
    "20260314": 2,  // INV: グランドセイコー エレガンス
    "20260315": 2,  // INV: ブライトリング ナビタイマー
  },

  // ===== 取引先会社 =====
  // ゲストアカウント・請求書宛先として使用
  clientCompanies: [
    {
      id: "CLI-001",
      companyName: "クロノス東京株式会社",
      tradeTypes: ["buyer", "supplier"],
      buyerCode: "B004",
      supplierCode: "S006",
      regionType: "domestic",
      representative: "田中 正雄",
      contactPerson: "田中 美穂",
      email: "info@chronos-tokyo.co.jp",
      tel: "03-9999-0000",
      contactPhone: "090-1111-2222",
      postalCode: "160-0023",
      address: "東京都新宿区西新宿2-1-1",
      invoice: "T0007777888899",
      antiqueLicenseNumber: "東京都公安委員会 第301020000001号",
      note: "",
      guestId: "G001",
    },
    {
      id: "CLI-002",
      companyName: "タイムレス商会有限会社",
      tradeTypes: ["buyer"],
      buyerCode: "B002",
      regionType: "domestic",
      representative: "中村 健一",
      contactPerson: "中村 由美",
      email: "info@timeless.co.jp",
      tel: "092-444-5555",
      contactPhone: "092-444-5556",
      postalCode: "812-0011",
      address: "福岡県福岡市博多区博多駅前3-2-8",
      invoice: "T0003333444455",
      antiqueLicenseNumber: "福岡県公安委員会 第901020000002号",
      note: "",
      guestId: "G002",
    },
  ],

  // ===== ダッシュボード管理設定 =====
  dashboardSettings: {
    salesTarget:    0,  // 今月売上目標金額（USD、数値・カンマなし）
    purchaseBudget: 0,  // 今月仕入予算額（数値・カンマなし）
  },

  // ===== 自社情報（全帳票・請求書・振込先の唯一の参照元） =====
  companyInfo: {
    companyName: "株式会社ウォッチプレミアム",
    zip: "〒105-0001",
    address: "東京都港区虎ノ門1-2-3 ウォッチビル5F",
    tel: "03-1234-0000",
    fax: "03-1234-0001",
    email: "info@watch-premium.co.jp",
    invoice: "T1234560000",
    bankName: "三菱UFJ銀行",
    branchName: "虎ノ門支店",
    accountType: "普通",
    accountNumber: "1234567",
    accountHolder: "カ）ウォッチプレミアム",
  },

  // ===== 仕入登録伝票 =====
  deletedPurchaseSlips: [],
  purchaseSlips: [
    {
      id: "PI-2026-0001",
      date: "2026-03-05",
      supplier: "S001",
      staff: "山本 太郎",
      note: "3月第1弾仕入れ",
      status: "処理済",
      registeredAt: "2026-03-05 10:30",
      revisions: [],
      lines: [
        {
          lineNo: 1, code: "20260305001", sku: "ROL-SUB-116610",
          purchasePrice: 820000, salePrice: 1180000,
          productDetail: {
            brand: "ロレックス", model: "サブマリーナ", ref: "116610LN",
            serial: "ZX223300", condition: "CON-003", accessories: ["BOX","GUARANTEE"]
          }
        },
        {
          lineNo: 2, code: "20260305002", sku: "OMG-SPD-311",
          purchasePrice: 310000, salePrice: 490000,
          productDetail: {
            brand: "オメガ", model: "スピードマスター", ref: "311.30.42.30",
            serial: "OM998877", condition: "CON-004", accessories: ["BOX"]
          }
        }
      ],
    },
    {
      id: "PI-2026-0002",
      date: "2026-04-10",
      supplier: "S002",
      staff: "佐藤 花子",
      note: "4月仕入れ・要確認あり",
      status: "承認待ち",
      approvalNote: "仕入価格の変更について確認が必要です。担当者より申請中。",
      approvalBy: "佐藤 花子",
      approvalAt: "2026-04-10 14:20",
      approvalChanges: [
        { field: "仕入金額（1行目）", before: "¥2,400,000", after: "¥2,600,000", reason: "交渉後の最終確定価格" }
      ],
      revisions: [
        { at: "2026-04-10 14:20", by: "佐藤 花子", note: "仕入金額を修正申請" }
      ],
      lines: [
        {
          lineNo: 1, code: "20260410001", sku: "PAT-AQU-5167",
          purchasePrice: 2600000, salePrice: 3500000,
          productDetail: {
            brand: "パテック・フィリップ", model: "アクアノート", ref: "5167A-001",
            serial: "PP556677", condition: "CON-002", accessories: ["BOX","GUARANTEE","CERTIFICATE"]
          }
        }
      ],
    },
    {
      id: "PI-2026-0003",
      date: "2026-04-14",
      supplier: "S003",
      staff: "鈴木 一郎",
      note: "",
      status: "未処理",
      revisions: [],
      lines: [
        {
          lineNo: 1, code: "20260414001", sku: "IWC-POR-IW500",
          purchasePrice: 450000, salePrice: 680000,
          productDetail: {
            brand: "IWC", model: "ポルトギーゼ", ref: "IW500704",
            serial: "IW334455", condition: "CON-004", accessories: ["BOX","GUARANTEE"]
          }
        },
        {
          lineNo: 2, code: "20260414002", sku: "BRL-NAV-AB012",
          purchasePrice: 390000, salePrice: 580000,
          productDetail: {
            brand: "ブライトリング", model: "ナビタイマー", ref: "AB0121",
            serial: "BR778899", condition: "CON-005", accessories: []
          }
        }
      ],
    },
    // ⑤ サンプル：承認待ち
    {
      id: "PI-2026-0004",
      date: "2026-04-15",
      supplier: "S004",
      staff: "田中 美香",
      note: "プラチナ時計 承認申請中",
      status: "承認待ち",
      approvalNote: "仕入金額の承認をお願いします。",
      approvalBy: "田中 美香",
      approvalAt: "2026-04-15 09:00",
      approvalChanges: [{ field: "仕入金額", before: "¥1,200,000", after: "¥1,350,000", reason: "最終交渉後の確定価格" }],
      revisions: [{ at: "2026-04-15 09:00", by: "田中 美香", note: "金額修正申請" }],
      lines: [{
        lineNo: 1, code: "20260415001", sku: "ROL-DATE-126300",
        purchasePrice: 1350000, salePrice: 1850000,
        productDetail: { brand: "ロレックス", model: "デイトジャスト", ref: "126300", serial: "RX445566", condition: "CON-002", accessories: ["BOX","GUARANTEE"] }
      }],
    },
    // ⑤ サンプル：差戻し
    {
      id: "PI-2026-0005",
      date: "2026-04-16",
      supplier: "S005",
      staff: "伊藤 健司",
      note: "差戻し対応中",
      status: "差戻し",
      revisionComment: "シリアル番号が不明瞭です。確認の上再申請してください。",
      revisions: [{ at: "2026-04-16 11:30", by: "伊藤 健司", note: "仕入登録申請 → 差戻し" }],
      lines: [{
        lineNo: 1, code: "20260416001", sku: "OMG-AQT-220",
        purchasePrice: 280000, salePrice: 420000,
        productDetail: { brand: "オメガ", model: "アクアテラ", ref: "220.10.38.20", serial: "", condition: "CON-004", accessories: ["BOX"] }
      }],
    },
    // ⑤ サンプル：差戻し②
    {
      id: "PI-2026-0006",
      date: "2026-04-17",
      supplier: "S002",
      staff: "山本 太郎",
      note: "",
      status: "差戻し",
      revisionComment: "コンディション評価が甘いです。再評価して再申請してください。",
      revisions: [{ at: "2026-04-17 10:00", by: "山本 太郎", note: "承認申請 → 差戻し" }],
      lines: [{
        lineNo: 1, code: "20260417001", sku: "PAT-CAL-5396",
        purchasePrice: 3200000, salePrice: 4500000,
        productDetail: { brand: "パテック・フィリップ", model: "カラトラバ", ref: "5396G-001", serial: "PP667788", condition: "CON-003", accessories: ["BOX","GUARANTEE","CERTIFICATE"] }
      }],
    },
  ],
};

// 既存サンプルの salePrice をUSDへ移行する。items配下に売価がある伝票は
// 合計も再計算し、一覧・詳細・CSV・承認画面で通貨が混在しないようにする。
function normalizeSeedSalePricesToUSD(node) {
  if (Array.isArray(node)) {
    node.forEach(normalizeSeedSalePricesToUSD);
    return;
  }
  if (!node || typeof node !== 'object') return;

  Object.entries(node).forEach(([key, value]) => {
    if ((key === 'salePrice' || key === 'wholesale') && typeof value === 'number') {
      node[key] = convertJPYToSalePriceUSD(value);
      return;
    }
    normalizeSeedSalePricesToUSD(value);
  });

  if (Array.isArray(node.items) && Object.prototype.hasOwnProperty.call(node, 'total')) {
    const saleItems = node.items.filter(item => item && (typeof item.salePrice === 'number' || typeof item.wholesale === 'number'));
    if (saleItems.length > 0) {
      node.total = saleItems.reduce((sum, item) => sum + (item.salePrice ?? item.wholesale ?? 0), 0);
    }
  }
}

normalizeSeedSalePricesToUSD(APP_DATA);

// =====================================================
// ユーティリティ関数
// =====================================================

function formatPrice(n) {
  if (n == null || n === '') return '—';
  return '¥' + Number(n).toLocaleString();
}

function convertJPYToSalePriceUSD(n) {
  const value = Number(n);
  if (!Number.isFinite(value) || value <= 0) return 0;
  return Math.round(value / SALE_PRICE_JPY_PER_USD);
}

function formatSalePrice(n) {
  if (n == null || n === '') return '—';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: SALE_PRICE_CURRENCY,
    maximumFractionDigits: 0,
  }).format(Number(n) || 0);
}

function formatDate(str) {
  if (!str) return '—';
  return str;
}

// 商品コード生成: YYYYMMDDNNN 形式
// dateStr: "YYYY-MM-DD" 形式の仕入日（省略時は今日）
// peek: true を渡すとカウンターを増やさずにプレビューを返す
function generateItemCode(dateStr, peek) {
  const d = dateStr ? new Date(dateStr) : new Date();
  const ymd = d.getFullYear().toString()
    + String(d.getMonth() + 1).padStart(2, '0')
    + String(d.getDate()).padStart(2, '0');

  if (!APP_DATA.itemNumberByDate) APP_DATA.itemNumberByDate = {};
  const current = APP_DATA.itemNumberByDate[ymd] || 1;
  const code = `${ymd}${String(current).padStart(3, '0')}`;
  if (!peek) {
    APP_DATA.itemNumberByDate[ymd] = current + 1;
  }
  return code;
}

function getSupplierName(code) {
  const s = APP_DATA.suppliers.find(x => x.code === code);
  return s ? s.name : code;
}

function getBuyerName(code) {
  const b = APP_DATA.buyers.find(x => x.code === code);
  return b ? b.name : code;
}

function getStatusBadge(status) {
  const map = {
    '在庫中': '<span class="badge badge-stock">● 在庫中</span>',
    '取置中': '<span class="badge badge-pending">● 取置中</span>',
    '売上済': '<span class="badge badge-sold">● 売上済</span>',
    '出荷済': '<span class="badge badge-shipped">● 出荷済</span>',
    '委託中': '<span class="badge badge-consigned">● 委託中</span>',
    '保留': '<span class="badge badge-pending">● 保留</span>',
  };
  return map[status] || `<span class="badge">${status}</span>`;
}

// BOX番号バッジを生成（在庫一覧など共通利用）
function _buildBoxBadge(boxNo) {
  if (boxNo == null) return '<span style="font-size:11px;color:var(--text-muted);">—</span>';
  const box   = (APP_DATA.boxes || []).find(b => b.no === boxNo);
  const label = box?.name?.trim() ? `BOX${boxNo}` : `BOX${boxNo}`;
  const title = box?.name?.trim() ? `${box.name}` : '';
  return `<span class="inv-box-badge" title="${title}" onclick="event.stopPropagation();openBoxLineupModal(${boxNo})">
    <i class="fa-solid fa-layer-group"></i> ${label}
  </span>`;
}

// =====================================================
// Toast 通知
// =====================================================
function showToast(type, title, msg, duration = 3000) {
  const icons = {
    success: '<i class="fa-solid fa-circle-check"></i>',
    error: '<i class="fa-solid fa-circle-xmark"></i>',
    info: '<i class="fa-solid fa-circle-info"></i>',
    warning: '<i class="fa-solid fa-triangle-exclamation"></i>'
  };
  const container = document.getElementById('toastContainer');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.innerHTML = `
    <div class="toast-icon">${icons[type] || icons.info}</div>
    <div class="toast-content">
      <div class="toast-title">${title}</div>
      ${msg ? `<div class="toast-msg">${msg}</div>` : ''}
    </div>
  `;
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.animation = 'fadeOut 0.3s ease forwards';
    setTimeout(() => toast.remove(), 300);
  }, duration);
}

// =====================================================
// =====================================================
// ページ遷移（SPA風）
// =====================================================
function navigateTo(page) {
  document.getElementById('pe-csv-error-toast')?.remove();
  // nav アイテムの active 状態更新
  document.querySelectorAll('.nav-item').forEach(el => {
    el.classList.toggle('active', el.dataset.page === page);
  });

  // ページコンテンツの表示切替
  document.querySelectorAll('.page-panel').forEach(el => {
    el.classList.add('hidden');
  });

  const target = document.getElementById('page-' + page);
  if (target) {
    target.classList.remove('hidden');
    // ページ初期化
    if (window['init_' + page]) window['init_' + page]();
  }

  // トップバー更新
  const pageNames = {
    'dashboard': 'ダッシュボード',
    'purchase': '商品登録',
    'sales': '売上登録',
    'shipping': '出荷登録',
    'master': 'マスタ登録',
    'performance': '実績管理',
    'inventory': '在庫一覧',
    'purchase-list': '購入一覧',
  };
  const titleEl = document.getElementById('pageTitle');
  if (titleEl) titleEl.textContent = pageNames[page] || page;
}
