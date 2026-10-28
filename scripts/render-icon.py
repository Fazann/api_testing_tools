"""Regenerates build/appicon.png and build/windows/icon.ico.

The geometry mirrors build/appicon.svg (the source of truth, also used as the
frontend favicon). Pillow is used instead of an SVG renderer so the output is
the same on every machine.

Usage: python3 scripts/render-icon.py   (requires: pip install pillow)
"""
from pathlib import Path

from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parent.parent
S = 2  # supersampling factor for smooth edges
N = 1024 * S
WHITE = (255, 255, 255, 255)


def p(x, y):
    return (x * S, y * S)


def bezier(p0, p1, p2, p3, steps=48):
    pts = []
    for i in range(1, steps + 1):
        t = i / steps
        u = 1 - t
        pts.append(
            tuple(u**3 * a + 3 * u * u * t * b + 3 * u * t * t * c + t**3 * d for a, b, c, d in zip(p0, p1, p2, p3))
        )
    return [p(*q) for q in pts]


def main():
    # Rounded square with a vertical #3b82f6 -> #1d4ed8 gradient.
    top, bottom = (0x3B, 0x82, 0xF6), (0x1D, 0x4E, 0xD8)
    grad = Image.new("RGBA", (N, N))
    gd = ImageDraw.Draw(grad)
    for y in range(N):
        t = y / (N - 1)
        gd.line([(0, y), (N, y)], fill=tuple(round(a + (b - a) * t) for a, b in zip(top, bottom)) + (255,))
    mask = Image.new("L", (N, N), 0)
    ImageDraw.Draw(mask).rounded_rectangle([p(64, 64), p(960, 960)], radius=208 * S, fill=255)
    img = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    img.paste(grad, (0, 0), mask)

    # Shield: M512 200 L772 292 V508 C772 668 660 784 512 840 C364 784 252 668 252 508 V292 Z
    shield = [p(512, 200), p(772, 292), p(772, 508)]
    shield += bezier((772, 508), (772, 668), (660, 784), (512, 840))
    shield += bezier((512, 840), (364, 784), (252, 668), (252, 508))
    shield += [p(252, 292)]
    overlay = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    ImageDraw.Draw(overlay).polygon(shield, fill=(255, 255, 255, 41))  # fill-opacity 0.16
    img = Image.alpha_composite(img, overlay)

    d = ImageDraw.Draw(img)
    d.line(shield + shield[:2], fill=WHITE, width=56 * S, joint="curve")

    # Check mark with round caps.
    check = [p(390, 520), p(478, 608), p(644, 442)]
    width = 72 * S
    d.line(check, fill=WHITE, width=width, joint="curve")
    r = width / 2
    for x, y in (check[0], check[-1]):
        d.ellipse([x - r, y - r, x + r, y + r], fill=WHITE)

    icon = img.resize((1024, 1024), Image.LANCZOS)
    icon.save(ROOT / "build/appicon.png", optimize=True)
    (ROOT / "build/windows").mkdir(parents=True, exist_ok=True)
    icon.save(ROOT / "build/windows/icon.ico", sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (256, 256)])


if __name__ == "__main__":
    main()
