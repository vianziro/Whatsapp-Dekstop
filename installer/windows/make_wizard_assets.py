#!/usr/bin/env python3
"""Generates the NSIS wizard graphics for the WhatsApp Desk Windows installer.

Visual language follows DESIGN.md: Ink (#111B21) surface, Action Green
(#00A884) for the single accent, muted slate (#8696A0) for supporting text,
system sans-serif type. Deliberately no gradients, glow or decorative cards.

Outputs (installer/windows/assets):
  welcome.bmp  164x314  first page sidebar
  header.bmp   150x57   page header strip
  icon.ico     (copied from repo root for the wizard + Add/Remove entry)

NSIS requires BMP3 for these slots; 24-bit BGR without alpha.
"""

import os
from PIL import Image, ImageDraw, ImageFont

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUT = os.path.join(REPO, "installer", "windows", "assets")

INK = (17, 27, 33)
LAYER = (32, 44, 51)
ACCENT = (0, 168, 132)
ACCENT_DEEP = (0, 128, 105)
MUTED = (134, 150, 160)
TEXT = (233, 237, 239)


def load_font(size, bold=False):
    """System sans that matches the app's font stack, with resilient fallback."""
    candidates = [
        "/System/Library/Fonts/SFNS.ttf",
        "/System/Library/Fonts/Supplemental/Arial Bold.ttf" if bold else "/System/Library/Fonts/Supplemental/Arial.ttf",
        "/System/Library/Fonts/Helvetica.ttc",
    ]
    for path in candidates:
        if os.path.exists(path):
            try:
                return ImageFont.truetype(path, size)
            except OSError:
                continue
    return ImageFont.load_default()


def app_icon(size):
    """Reuse the shipped app icon so installer and app share one identity."""
    for name in ("icon_1024.png", "icon.png"):
        path = os.path.join(REPO, name)
        if os.path.exists(path):
            im = Image.open(path).convert("RGBA")
            return im.resize((size, size), Image.LANCZOS)
    return None


def rounded_rect(draw, box, radius, fill=None, outline=None, width=1):
    draw.rounded_rectangle(box, radius=radius, fill=fill, outline=outline, width=width)


def build_welcome():
    """164x314 sidebar: icon, product name, one-line promise, version chip."""
    w, h = 164, 314
    img = Image.new("RGB", (w, h), INK)
    d = ImageDraw.Draw(img)

    # A single quiet band at the bottom grounds the panel without decoration.
    d.rectangle([0, h - 6, w, h], fill=ACCENT_DEEP)

    icon_size = 64
    icon = app_icon(icon_size)
    if icon is not None:
        img.paste(icon, ((w - icon_size) // 2, 52), icon)

    title_font = load_font(16, bold=True)
    body_font = load_font(11)
    tiny_font = load_font(10)

    title = "WhatsApp Desk"
    tw = d.textlength(title, font=title_font)
    d.text(((w - tw) / 2, 134), title, font=title_font, fill=TEXT)

    tagline = "WhatsApp Web, as a desktop app"
    # Wrap the tagline to the panel width instead of overflowing.
    words, lines, cur = tagline.split(), [], ""
    for word in words:
        probe = (cur + " " + word).strip()
        if d.textlength(probe, font=body_font) <= w - 28:
            cur = probe
        else:
            lines.append(cur)
            cur = word
    if cur:
        lines.append(cur)
    y = 158
    for line in lines:
        lw = d.textlength(line, font=body_font)
        d.text(((w - lw) / 2, y), line, font=body_font, fill=MUTED)
        y += 15

    # Version chip: reads as metadata, not as a marketing badge.
    chip = "v" + os.environ.get("WA_DESK_VERSION", "1.5.9.7")
    cw = d.textlength(chip, font=tiny_font) + 18
    cx0 = (w - cw) / 2
    rounded_rect(d, [cx0, 198, cx0 + cw, 220], radius=11, fill=LAYER)
    d.text(((w - d.textlength(chip, font=tiny_font)) / 2, 203), chip, font=tiny_font, fill=ACCENT)

    return img


def build_header():
    """150x57 header strip shown on every wizard page."""
    w, h = 150, 57
    img = Image.new("RGB", (w, h), INK)
    d = ImageDraw.Draw(img)

    icon_size = 32
    icon = app_icon(icon_size)
    if icon is not None:
        img.paste(icon, (w - icon_size - 10, (h - icon_size) // 2), icon)

    name_font = load_font(12, bold=True)
    sub_font = load_font(9)
    d.text((12, 15), "WhatsApp Desk", font=name_font, fill=TEXT)
    d.text((12, 32), "Installer", font=sub_font, fill=MUTED)

    # Thin accent rule ties the header to the sidebar accent.
    d.rectangle([0, h - 3, w, h], fill=ACCENT)

    return img


def save_bmp(img, path):
    """NSIS wants BMP3: 24-bit, no alpha, no color profile."""
    img.convert("RGB").save(path, format="BMP")


def main():
    os.makedirs(OUT, exist_ok=True)
    save_bmp(build_welcome(), os.path.join(OUT, "welcome.bmp"))
    save_bmp(build_header(), os.path.join(OUT, "header.bmp"))

    ico_src = os.path.join(REPO, "icon.ico")
    ico_dst = os.path.join(OUT, "icon.ico")
    if os.path.exists(ico_src):
        with open(ico_src, "rb") as src, open(ico_dst, "wb") as dst:
            dst.write(src.read())

    for name in ("welcome.bmp", "header.bmp", "icon.ico"):
        p = os.path.join(OUT, name)
        if os.path.exists(p):
            print(f"{name}: {os.path.getsize(p)} bytes")


if __name__ == "__main__":
    main()
