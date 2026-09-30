"""Builds every brand asset (logo, favicons, social image) from the full-size logo file.

Usage: python3 brand/generate.py path/to/logo-1536.png
Needs Pillow, numpy and scipy. Outputs go to public/ and src/assets/; the transparent master is kept in brand/.
"""
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter, ImageFont
from scipy import ndimage

ROOT = Path(__file__).resolve().parent.parent
PUBLIC, ASSETS, BRAND = ROOT / "public", ROOT / "src" / "assets" / "brand", ROOT / "brand"
SITE = "layr.appmd.dev"


def extract_mark(path):
    """Cuts the mark out of its off-white background and returns an RGBA image with smooth edges."""
    rgb = np.asarray(Image.open(path).convert("RGB")).astype(np.int16)
    distance = (255 - rgb).max(axis=2)
    mask = distance > 14
    mask = ndimage.binary_fill_holes(ndimage.binary_opening(mask, iterations=2))
    labels, count = ndimage.label(mask)
    if count > 1:
        sizes = ndimage.sum(mask, labels, range(1, count + 1))
        mask = labels == (1 + int(np.argmax(sizes)))
    mask = ndimage.binary_erosion(mask, iterations=1)
    alpha = Image.fromarray((mask * 255).astype(np.uint8)).filter(ImageFilter.GaussianBlur(1.1))
    out = Image.open(path).convert("RGBA")
    out.putalpha(alpha)
    return out.crop(out.getbbox())


def fit(mark, box_w, box_h):
    scale = min(box_w / mark.width, box_h / mark.height)
    return mark.resize((max(1, round(mark.width * scale)), max(1, round(mark.height * scale))), Image.LANCZOS)


def square(mark, size, pad, background=None):
    canvas = Image.new("RGBA", (size, size), background or (0, 0, 0, 0))
    inner = round(size * (1 - 2 * pad))
    m = fit(mark, inner, inner)
    canvas.alpha_composite(m, ((size - m.width) // 2, (size - m.height) // 2))
    return canvas


def bold_font(size):
    for path in ("/System/Library/Fonts/HelveticaNeue.ttc", "/System/Library/Fonts/Helvetica.ttc", "/Library/Fonts/Arial Unicode.ttf"):
        for index in range(12):
            try:
                font = ImageFont.truetype(path, size, index=index)
            except Exception:
                break
            if "Bold" in " ".join(font.getname()):
                return font
    return ImageFont.load_default(size)


def gradient(width, height, stops):
    xs = np.linspace(0, 1, width)
    channels = []
    for c in range(3):
        channels.append(np.interp(xs, [s[0] for s in stops], [s[1][c] for s in stops]))
    row = np.stack(channels, axis=1).astype(np.uint8)
    return Image.fromarray(np.repeat(row[None, :, :], height, axis=0))


def blob(canvas, box, colors):
    x0, y0, x1, y1 = box
    layer = gradient(x1 - x0, y1 - y0, [(0, colors[0]), (1, colors[1])]).convert("RGBA")
    mask = Image.new("L", layer.size, 0)
    ImageDraw.Draw(mask).ellipse((0, 0, layer.width - 1, layer.height - 1), fill=255)
    layer.putalpha(mask)
    canvas.alpha_composite(layer, (x0, y0))


def spark(draw, cx, cy, r, fill):
    pts = []
    for i, (dx, dy) in enumerate([(0, -1), (0.22, -0.22), (1, 0), (0.22, 0.22), (0, 1), (-0.22, 0.22), (-1, 0), (-0.22, -0.22)]):
        pts.append((cx + dx * r, cy + dy * r))
    draw.polygon(pts, fill=fill)


def social_image(mark):
    """1200x630 share card, drawn at 2x and reduced for clean edges."""
    w, h = 2400, 1260
    card = Image.new("RGBA", (w, h), (255, 255, 255, 255))
    blob(card, (-520, -520, 560, 480), ((181, 211, 255), (170, 161, 247)))
    blob(card, (-360, 780, 900, 1560), ((210, 243, 239), (210, 255, 240)))
    blob(card, (1780, 820, 2700, 1560), ((188, 205, 255), (248, 245, 255)))
    blob(card, (2120, 120, 2560, 620), ((210, 243, 239), (210, 255, 240)))
    d = ImageDraw.Draw(card)
    spark(d, 2010, 250, 34, (89, 58, 250, 255))
    spark(d, 470, 1010, 26, (85, 239, 199, 255))
    d.ellipse((2080, 1010, 2116, 1046), fill=(89, 58, 250, 255))

    logo = fit(mark, 300, 300)
    card.alpha_composite(logo, ((w - logo.width) // 2, 170))

    head = bold_font(158)
    line1, line2 = "Turn your designs", "into real code"
    tw = d.textlength(line1, font=head)
    d.text(((w - tw) / 2, 540), line1, font=head, fill=(16, 17, 31, 255))
    tw2 = d.textlength(line2, font=head)
    mask = Image.new("L", (w, h), 0)
    ImageDraw.Draw(mask).text(((w - tw2) / 2, 720), line2, font=head, fill=255)
    fill = gradient(w, h, [(0.30, (48, 99, 251)), (0.45, (21, 147, 245)), (0.62, (33, 220, 194)), (0.72, (29, 231, 174))]).convert("RGBA")
    card.paste(fill, (0, 0), mask)

    sub = bold_font(58)
    line = "Connect Figma. Get production-ready code."
    sw = d.textlength(line, font=sub)
    d.text(((w - sw) / 2, 990), line, font=sub, fill=(133, 132, 152, 255))
    small = bold_font(44)
    sw = d.textlength(SITE, font=small)
    d.text(((w - sw) / 2, 1120), SITE, font=small, fill=(129, 128, 147, 255))
    return card.convert("RGB").resize((1200, 630), Image.LANCZOS)


def main(source):
    for folder in (PUBLIC, ASSETS, BRAND):
        folder.mkdir(parents=True, exist_ok=True)
    mark = extract_mark(source)
    mark.save(BRAND / "logo-master.png", optimize=True)

    tall = fit(mark, 400, 400)
    tall.save(ASSETS / "logo.webp", "WEBP", quality=90, method=6)

    icons = {size: square(mark, size, 0.07) for size in (16, 32, 48)}
    for size in (16, 32):
        icons[size].save(PUBLIC / f"favicon-{size}x{size}.png", optimize=True)
    icons[48].save(PUBLIC / "favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)])
    square(mark, 180, 0.18, (255, 255, 255, 255)).convert("RGB").save(PUBLIC / "apple-touch-icon.png", optimize=True)
    square(mark, 192, 0.10).save(PUBLIC / "icon-192.png", optimize=True)
    square(mark, 512, 0.10).save(PUBLIC / "icon-512.png", optimize=True)
    square(mark, 512, 0.24, (255, 255, 255, 255)).convert("RGB").save(PUBLIC / "icon-maskable-512.png", optimize=True)

    social_image(mark).save(PUBLIC / "og-image.png", optimize=True)
    for path in sorted(list(PUBLIC.glob("*.png")) + list(PUBLIC.glob("*.ico")) + list(ASSETS.glob("*.webp"))):
        print(f"{path.relative_to(ROOT)}  {path.stat().st_size} bytes")


if __name__ == "__main__":
    main(sys.argv[1])
