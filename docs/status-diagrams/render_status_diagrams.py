from pathlib import Path
from PIL import Image, ImageDraw, ImageFont
import math


OUT = Path(__file__).resolve().parent
FONT_REGULAR = r"C:\Windows\Fonts\meiryo.ttc"
FONT_BOLD = r"C:\Windows\Fonts\meiryob.ttc"

COLORS = {
    "ink": "#173B5F",
    "line": "#557087",
    "muted": "#6F8293",
    "blue_fill": "#EAF4FB",
    "blue_border": "#2E86BD",
    "orange_fill": "#FFF4DF",
    "orange_border": "#ED8324",
    "green_fill": "#EDF7EF",
    "green_border": "#35A66F",
    "red_fill": "#FDECEC",
    "red_border": "#D55353",
    "gray_fill": "#F2F5F7",
    "gray_border": "#9AAAB7",
}


def font(size, bold=False):
    return ImageFont.truetype(FONT_BOLD if bold else FONT_REGULAR, size)


TITLE = font(44, True)
NODE = font(27, False)
NODE_BOLD = font(27, True)
EDGE = font(22, True)
LEGEND = font(20, False)


def text_size(draw, text, fnt):
    box = draw.multiline_textbbox((0, 0), text, font=fnt, spacing=6, align="center")
    return box[2] - box[0], box[3] - box[1]


def draw_node(draw, center, size, text, tone="blue", kind="box", bold=False):
    cx, cy = center
    w, h = size
    fill = COLORS[f"{tone}_fill"]
    border = COLORS[f"{tone}_border"]
    if kind == "diamond":
        pts = [(cx, cy - h / 2), (cx + w / 2, cy), (cx, cy + h / 2), (cx - w / 2, cy)]
        draw.polygon(pts, fill=fill, outline=border, width=4)
    else:
        draw.rounded_rectangle((cx - w / 2, cy - h / 2, cx + w / 2, cy + h / 2), radius=18, fill=fill, outline=border, width=4)
    fnt = NODE_BOLD if bold else NODE
    tw, th = text_size(draw, text, fnt)
    draw.multiline_text((cx - tw / 2, cy - th / 2), text, font=fnt, fill=COLORS["ink"], spacing=6, align="center")


def arrow_head(draw, start, end, color=None):
    color = color or COLORS["line"]
    angle = math.atan2(end[1] - start[1], end[0] - start[0])
    length = 17
    spread = math.radians(28)
    p1 = (end[0] - length * math.cos(angle - spread), end[1] - length * math.sin(angle - spread))
    p2 = (end[0] - length * math.cos(angle + spread), end[1] - length * math.sin(angle + spread))
    draw.polygon([end, p1, p2], fill=color)


def draw_edge(draw, points, label=None, label_at=None, dashed=False, color=None):
    color = color or COLORS["line"]
    if dashed:
        for a, b in zip(points, points[1:]):
            distance = math.dist(a, b)
            if distance == 0:
                continue
            steps = max(1, int(distance / 20))
            for i in range(0, steps, 2):
                t1 = i / steps
                t2 = min((i + 1) / steps, 1)
                p1 = (a[0] + (b[0] - a[0]) * t1, a[1] + (b[1] - a[1]) * t1)
                p2 = (a[0] + (b[0] - a[0]) * t2, a[1] + (b[1] - a[1]) * t2)
                draw.line([p1, p2], fill=color, width=4)
    else:
        draw.line(points, fill=color, width=4, joint="curve")
    arrow_head(draw, points[-2], points[-1], color)
    if label:
        if label_at is None:
            a, b = points[len(points) // 2 - 1], points[len(points) // 2]
            label_at = ((a[0] + b[0]) / 2, (a[1] + b[1]) / 2)
        box = draw.textbbox((0, 0), label, font=EDGE)
        tw, th = box[2] - box[0], box[3] - box[1]
        x, y = label_at
        draw.rounded_rectangle((x - tw / 2 - 8, y - th / 2 - 5, x + tw / 2 + 8, y + th / 2 + 5), radius=7, fill="white")
        draw.text((x - tw / 2, y - th / 2), label, font=EDGE, fill=COLORS["ink"])


def title(draw, text):
    draw.text((50, 36), text, font=TITLE, fill=COLORS["ink"])
    draw.line((50, 100, 650, 100), fill=COLORS["orange_border"], width=7)


def legend(draw, y):
    items = [
        ("blue", "通常処理"),
        ("orange", "待機・進行中"),
        ("green", "完了・利用可能"),
        ("red", "取消・例外・要対応"),
        ("gray", "補助処理"),
    ]
    x = 50
    for tone, label in items:
        draw.rounded_rectangle((x, y, x + 28, y + 28), radius=5, fill=COLORS[f"{tone}_fill"], outline=COLORS[f"{tone}_border"], width=3)
        draw.text((x + 38, y - 1), label, font=LEGEND, fill=COLORS["muted"])
        x += 190


def make_purchase():
    img = Image.new("RGB", (1900, 1660), "white")
    d = ImageDraw.Draw(img)
    title(d, "1. 仕入から在庫化まで")

    n = {
        "p0": (650, 180), "p1": (650, 340), "p2": (650, 500), "p3": (650, 660),
        "p4": (650, 830), "p5": (650, 1010), "p7": (1370, 530),
        "p6": (650, 1190), "p8": (1110, 1190), "p9": (1110, 1420),
        "p10": (360, 1420), "p11": (650, 1420),
    }
    size = (390, 116)
    draw_node(d, n["p0"], size, "仕入内容を入力\n伝票：下書き")
    draw_node(d, n["p1"], size, "作業者が申請\n伝票：承認待ち", "orange")
    draw_node(d, n["p2"], size, "管理者が承認\n伝票：確定", "blue")
    draw_node(d, n["p3"], size, "未納\n商品：入荷待ち／所在：仕入先", "orange")
    draw_node(d, n["p4"], size, "一部納品\n納品分：在庫中／残数：入荷待ち", "orange")
    draw_node(d, n["p5"], (430, 130), "全数納品・検品完了\n伝票：完了／商品：在庫中\n所在：自社", "green", bold=True)
    draw_node(d, n["p7"], size, "仕入取消\n伝票：取消", "red")
    draw_node(d, n["p6"], size, "検品不合格\n商品：検品保留", "red")
    draw_node(d, n["p8"], size, "仕入返品伝票を起票\n商品：仕入返品処理中", "orange")
    draw_node(d, n["p9"], size, "仕入先へ返送\n商品：仕入返品処理済", "gray")
    draw_node(d, n["p10"], (300, 100), "修理・再検品", "gray")
    draw_node(d, n["p11"], (300, 100), "廃棄処理", "red")

    draw_edge(d, [(650, 238), (650, 282)])
    draw_edge(d, [(650, 398), (650, 442)], "承認", (725, 420))
    draw_edge(d, [(650, 558), (650, 602)])
    draw_edge(d, [(650, 718), (650, 772)], "一部受入", (735, 745))
    draw_edge(d, [(650, 888), (650, 945)], "残数受入", (735, 918))
    draw_edge(d, [(845, 340), (1220, 340), (1220, 475)], "却下", (1110, 340))
    draw_edge(d, [(845, 660), (1370, 660), (1370, 588)], "発注取消", (1110, 660))
    draw_edge(d, [(650, 1075), (650, 1132)], "不良発見", (735, 1104))
    draw_edge(d, [(845, 1010), (1110, 1010), (1110, 1132)], "仕入返品", (1010, 1010))
    draw_edge(d, [(1110, 1248), (1110, 1362)])
    draw_edge(d, [(520, 1248), (360, 1248), (360, 1370)], "修理", (410, 1248))
    draw_edge(d, [(650, 1248), (650, 1370)], "再販不可", (740, 1305))
    draw_edge(d, [(520, 1190), (245, 1190), (245, 1010), (430, 1010)], "再販可能", (245, 1110), dashed=True)
    draw_edge(d, [(360, 1370), (360, 1040), (430, 1040)], "修理完了", (360, 1100), dashed=True)
    legend(d, 1585)
    return img


def make_sales():
    img = Image.new("RGB", (2300, 2060), "white")
    d = ImageDraw.Draw(img)
    title(d, "2. 購入申請・取置・出荷・売上")
    box = (380, 110)
    n = {
        "i": (1000, 180), "q": (1000, 340), "r": (1000, 500), "rj": (1700, 340),
        "s0": (430, 690), "s1": (430, 850), "s2": (220, 1020), "s3": (640, 1020),
        "s4": (640, 1190), "s5": (220, 1190), "h0": (1500, 730), "h1": (1900, 910),
        "v0": (1150, 1390), "v1": (1150, 1550), "v2": (780, 1730), "v3": (1150, 1900),
        "v4": (1510, 1730), "v5": (1840, 1900),
    }
    draw_node(d, n["i"], box, "商品：在庫中\n所在：自社", "green", bold=True)
    draw_node(d, n["q"], box, "ゲスト購入リクエスト\n承認待ち", "orange")
    draw_node(d, n["r"], box, "管理者承認\n商品：取置中", "blue", bold=True)
    draw_node(d, n["rj"], box, "却下・期限切れ・取消\n商品：在庫中へ戻す", "red")
    draw_node(d, n["s0"], box, "出荷伝票：下書き")
    draw_node(d, n["s1"], box, "出荷伝票：確定\n商品：出荷予定", "orange")
    draw_node(d, n["s2"], (330, 100), "一部出荷", "orange")
    draw_node(d, n["s3"], (330, 100), "出荷済\n所在：輸送中", "blue")
    draw_node(d, n["s4"], (330, 100), "納品完了\n所在：顧客", "green")
    draw_node(d, n["s5"], (330, 100), "持ち帰り・受取拒否\n返品処理中", "red")
    draw_node(d, n["h0"], box, "商品は自社に保管\n商品：引渡待ち", "orange")
    draw_node(d, n["h1"], box, "店頭・自社で\n引渡し完了", "green")
    draw_node(d, n["v0"], box, "売上伝票：下書き")
    draw_node(d, n["v1"], box, "売上伝票：確定\n入金待ち", "orange")
    draw_node(d, n["v2"], (320, 100), "一部入金", "orange")
    draw_node(d, n["v3"], (320, 100), "入金済", "green")
    draw_node(d, n["v4"], (320, 100), "支払期限超過", "red")
    draw_node(d, n["v5"], (320, 100), "貸倒処理", "red")

    draw_edge(d, [(1000, 235), (1000, 285)])
    draw_edge(d, [(1000, 395), (1000, 445)], "承認", (1070, 420))
    draw_edge(d, [(1190, 340), (1510, 340)], "却下・取消", (1350, 310))
    draw_edge(d, [(1700, 395), (1700, 500), (1190, 500)], "在庫へ戻す", (1480, 500))
    draw_edge(d, [(810, 500), (430, 500), (430, 635)], "出荷へ", (610, 500))
    draw_edge(d, [(430, 745), (430, 795)])
    draw_edge(d, [(430, 905), (220, 905), (220, 970)], "一部のみ", (290, 905))
    draw_edge(d, [(430, 905), (640, 905), (640, 970)], "全数出荷", (575, 905))
    draw_edge(d, [(385, 1020), (475, 1020)], "残数出荷", (430, 985))
    draw_edge(d, [(640, 1070), (640, 1140)], "配送完了", (730, 1105))
    draw_edge(d, [(475, 1020), (220, 1020), (220, 1140)], "配送失敗", (320, 1055))
    draw_edge(d, [(1190, 500), (1500, 500), (1500, 675)], "自社保管", (1370, 500))
    draw_edge(d, [(1690, 730), (1900, 730), (1900, 855)], "引渡し", (1820, 730))
    draw_edge(d, [(640, 1240), (640, 1390), (960, 1390)], "納品後売上", (760, 1390))
    draw_edge(d, [(1500, 785), (1500, 1390), (1340, 1390)], "自社保管売上", (1500, 1250))
    draw_edge(d, [(1000, 555), (1000, 1335)], "出荷前請求", (1000, 1230), dashed=True)
    draw_edge(d, [(1150, 1445), (1150, 1495)])
    draw_edge(d, [(960, 1550), (780, 1550), (780, 1680)], "一部入金", (850, 1550))
    draw_edge(d, [(1150, 1605), (1150, 1850)], "全額入金", (1230, 1740))
    draw_edge(d, [(940, 1730), (1150, 1730), (1150, 1850)], "残額入金", (1080, 1695))
    draw_edge(d, [(1340, 1550), (1510, 1550), (1510, 1680)], "期限超過", (1440, 1515))
    draw_edge(d, [(940, 1730), (1510, 1730)], "期限超過", (1230, 1695), dashed=True)
    draw_edge(d, [(1670, 1730), (1840, 1730), (1840, 1850)], "回収不能", (1780, 1695))
    draw_edge(d, [(1510, 1780), (1510, 1900), (1310, 1900)], "回収", (1510, 1850), dashed=True)
    legend(d, 1990)
    return img


def make_returns():
    img = Image.new("RGB", (2200, 1730), "white")
    d = ImageDraw.Draw(img)
    title(d, "3. 返品・交換・返金")
    box = (380, 110)
    n = {
        "a": (1050, 170), "b": (1050, 350), "c": (350, 540), "d": (1050, 540),
        "e": (1050, 720), "f": (650, 910), "g": (1150, 910), "h": (1150, 1070),
        "i": (1150, 1230), "j": (1150, 1410), "k": (500, 1600), "l": (900, 1600),
        "m": (1300, 1600), "n": (1710, 1600), "o": (1780, 540), "p": (1780, 760),
        "q": (1780, 980), "r": (1780, 1200), "s": (1780, 1410),
    }
    draw_node(d, n["a"], box, "返品受付", "orange", bold=True)
    draw_node(d, n["b"], (430, 130), "売上伝票は\n確定済みか", "blue", "diamond", True)
    draw_node(d, n["c"], box, "売上前取消\n取置・出荷を解除", "red")
    draw_node(d, n["d"], box, "売上返品伝票を起票", "blue", bold=True)
    draw_node(d, n["e"], (430, 130), "商品は\nどこにあるか", "blue", "diamond", True)
    draw_node(d, n["f"], box, "商品は自社\n物理的な返送なし", "gray")
    draw_node(d, n["g"], box, "商品は顧客\n返品処理中", "orange")
    draw_node(d, n["h"], box, "商品返送中", "orange")
    draw_node(d, n["i"], box, "自社で返品受領・検品", "blue")
    draw_node(d, n["j"], (430, 130), "検品結果", "blue", "diamond", True)
    draw_node(d, n["k"], (320, 100), "再販可能\n商品：在庫中", "green")
    draw_node(d, n["l"], (320, 100), "修理可能\n商品：修理中", "orange")
    draw_node(d, n["m"], (320, 100), "再販不可\n商品：廃棄", "red")
    draw_node(d, n["n"], (350, 100), "交換\n代替商品を取置・出荷", "blue")
    draw_node(d, n["o"], (360, 120), "入金状態", "blue", "diamond", True)
    draw_node(d, n["p"], (330, 100), "未入金\n請求取消", "gray")
    draw_node(d, n["q"], (330, 100), "一部入金\n差額調整・返金", "orange")
    draw_node(d, n["r"], (330, 100), "入金済\n返金待ち", "orange")
    draw_node(d, n["s"], (330, 100), "返金済", "green")

    draw_edge(d, [(1050, 225), (1050, 285)])
    draw_edge(d, [(835, 350), (350, 350), (350, 485)], "未確定", (600, 350))
    draw_edge(d, [(1050, 415), (1050, 485)], "確定済み", (1140, 450))
    draw_edge(d, [(1050, 595), (1050, 655)])
    draw_edge(d, [(835, 720), (650, 720), (650, 855)], "自社", (730, 720))
    draw_edge(d, [(1265, 720), (1150, 720), (1150, 855)], "顧客", (1220, 685))
    draw_edge(d, [(1150, 965), (1150, 1015)])
    draw_edge(d, [(1150, 1125), (1150, 1175)])
    draw_edge(d, [(650, 965), (650, 1230), (960, 1230)], "そのまま検品", (650, 1130))
    draw_edge(d, [(1150, 1285), (1150, 1345)])
    draw_edge(d, [(935, 1410), (500, 1410), (500, 1550)], "再販可", (700, 1410))
    draw_edge(d, [(1050, 1475), (900, 1475), (900, 1550)], "修理", (960, 1505))
    draw_edge(d, [(1250, 1475), (1300, 1475), (1300, 1550)], "再販不可", (1390, 1475))
    draw_edge(d, [(1365, 1410), (1710, 1410), (1710, 1550)], "交換希望", (1540, 1410))
    draw_edge(d, [(1050, 540), (1600, 540)], "入金を確認", (1450, 505), dashed=True)
    draw_edge(d, [(1780, 600), (1780, 710)], "未入金", (1860, 655))
    draw_edge(d, [(1780, 810), (1780, 930)], "一部入金", (1870, 870))
    draw_edge(d, [(1780, 1030), (1780, 1150)], "入金済", (1860, 1090))
    draw_edge(d, [(1780, 1250), (1780, 1360)])
    draw_edge(d, [(900, 1550), (900, 1475), (1050, 1475)], "修理完了", (900, 1510), dashed=True)
    legend(d, 1660)
    return img


def make_special():
    img = Image.new("RGB", (2300, 1420), "white")
    d = ImageDraw.Draw(img)
    title(d, "4. 特殊な在庫移動")
    draw_node(d, (250, 230), (320, 100), "在庫中", "green", bold=True)

    lane_y = [370, 650, 930, 1210]
    lane_titles = ["委託販売", "貸出・展示", "倉庫移動", "棚卸差異"]
    for y, label in zip(lane_y, lane_titles):
        d.text((50, y - 15), label, font=NODE_BOLD, fill=COLORS["ink"])
        d.line((210, y + 20, 2250, y + 20), fill="#DCE4EA", width=3)

    draw_node(d, (600, 370), (370, 110), "委託出庫", "blue")
    draw_node(d, (1100, 370), (420, 120), "委託中\n所在：委託先／所有権：自社", "orange")
    draw_node(d, (1700, 300), (360, 100), "委託販売成立\n売上済", "green")
    draw_node(d, (1700, 460), (360, 100), "売れずに返却\n在庫中", "gray")
    draw_edge(d, [(410, 230), (500, 230), (500, 370), (415, 370)], "委託", (500, 285))
    draw_edge(d, [(785, 370), (890, 370)])
    draw_edge(d, [(1310, 350), (1520, 300)], "販売成立", (1410, 300))
    draw_edge(d, [(1310, 400), (1520, 460)], "返却", (1410, 450))

    draw_node(d, (600, 650), (370, 110), "貸出・展示", "blue")
    draw_node(d, (1100, 650), (420, 120), "貸出中\n所在：相手先／所有権：自社", "orange")
    draw_node(d, (1700, 580), (360, 100), "返却・検品OK\n在庫中", "green")
    draw_node(d, (1700, 720), (360, 100), "破損・紛失\n検品保留", "red")
    draw_edge(d, [(410, 230), (460, 230), (460, 650), (415, 650)], "貸出", (460, 545))
    draw_edge(d, [(785, 650), (890, 650)])
    draw_edge(d, [(1310, 630), (1520, 580)], "正常返却", (1410, 580))
    draw_edge(d, [(1310, 670), (1520, 720)], "事故", (1410, 720))

    draw_node(d, (600, 930), (370, 110), "倉庫移動伝票", "blue")
    draw_node(d, (1100, 930), (360, 100), "移動中", "orange")
    draw_node(d, (1700, 930), (400, 110), "在庫中\n所在：移動先倉庫", "green")
    draw_edge(d, [(410, 230), (420, 230), (420, 930), (415, 930)], "移動", (420, 820))
    draw_edge(d, [(785, 930), (920, 930)])
    draw_edge(d, [(1280, 930), (1500, 930)], "移動完了", (1390, 890))

    draw_node(d, (600, 1210), (370, 110), "棚卸差異を登録", "blue")
    draw_node(d, (1100, 1210), (360, 100), "棚卸保留", "orange")
    draw_node(d, (1700, 1125), (360, 100), "現物確認\n在庫中へ復帰", "green")
    draw_node(d, (1700, 1245), (360, 100), "不足確定\n紛失処理", "red")
    draw_node(d, (1700, 1360), (360, 90), "過剰在庫\n在庫追加", "gray")
    draw_edge(d, [(410, 230), (380, 230), (380, 1210), (415, 1210)], "棚卸", (380, 1100))
    draw_edge(d, [(785, 1210), (920, 1210)])
    draw_edge(d, [(1280, 1190), (1520, 1125)], "現物確認", (1410, 1125))
    draw_edge(d, [(1280, 1210), (1520, 1245)], "不足", (1410, 1245))
    draw_edge(d, [(1280, 1230), (1450, 1360), (1520, 1360)], "過剰", (1410, 1320))
    legend(d, 1355)
    return img


def save_all():
    images = [
        ("01-purchase-to-inventory.png", make_purchase()),
        ("02-request-shipping-sales.png", make_sales()),
        ("03-return-exchange-refund.png", make_returns()),
        ("04-special-inventory-movements.png", make_special()),
    ]
    for name, image in images:
        image.save(OUT / name, format="PNG", optimize=True)

    target_width = 1800
    resized = []
    for _, image in images:
        scale = target_width / image.width
        resized.append(image.resize((target_width, round(image.height * scale)), Image.Resampling.LANCZOS))
    gap = 40
    combined = Image.new("RGB", (target_width, sum(im.height for im in resized) + gap * (len(resized) - 1)), "white")
    y = 0
    for im in resized:
        combined.paste(im, (0, y))
        y += im.height + gap
    combined.save(OUT / "00-all-status-diagrams.png", format="PNG", optimize=True)


if __name__ == "__main__":
    save_all()
