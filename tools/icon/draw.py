"""Draws Althea's icon: a mallow flower (Althaea officinalis), five overlapping heart-shaped
petals around a magenta column of stamens, on the blue-to-violet rounded square.

    python tools/icon/draw.py

writes resources/icon.png (1024 px, the app's) and internal/shell/tray.png (32 px). Needs Pillow.
"""
import math, sys
from PIL import Image, ImageDraw, ImageFilter

N = 4096            # drawn at 4x, then scaled down to 1024
k = N / 1024
out = "resources/icon.png"

def lerp(a, b, t): return tuple(round(a[i] + (b[i] - a[i]) * t) for i in range(len(a)))

# Background: the rounded square of the current icon, blue to violet.
bg = Image.new("RGBA", (N, N))
c0, c1 = (79, 123, 247), (155, 92, 246)
grad = Image.new("RGBA", (N, N))
gd = ImageDraw.Draw(grad)
for i in range(0, 2 * N, 8):
    t = i / (2 * N)
    gd.line([(i, 0), (0, i)], fill=lerp(c0, c1, t) + (255,), width=12)
mask = Image.new("L", (N, N))
ImageDraw.Draw(mask).rounded_rectangle([100 * k, 100 * k, 924 * k, 924 * k], radius=185 * k, fill=255)
bg.paste(grad, (0, 0), mask)

# A petal: a heart, its point at the flower's center and its notch at the tip.
def petal(angle, length):
    pts = []
    for s in range(720):
        t = 2 * math.pi * s / 720
        x = 16 * math.sin(t) ** 3
        y = 13 * math.cos(t) - 5 * math.cos(2 * t) - 2 * math.cos(3 * t) - math.cos(4 * t)
        y += 17                      # the point (y = -17) to the origin
        x, y = x / 29 * length, y / 29 * length
        a = math.radians(angle)
        px = x * math.cos(a) - y * math.sin(a)
        py = x * math.sin(a) + y * math.cos(a)
        pts.append((512 * k + px, 512 * k - py))
    return pts

length = 300 * k
petals = Image.new("L", (N, N))
pd = ImageDraw.Draw(petals)
for i in range(5):
    pd.polygon(petal(i * 72 + 18, length), fill=255)

# A soft shadow under the petals, then the petals: white, blushing pink towards the center.
shadow = petals.filter(ImageFilter.GaussianBlur(28 * k))
shade = Image.new("RGBA", (N, N), (60, 30, 140, 0))
shade.putalpha(shadow.point(lambda v: v * 0.35))
bg = Image.alpha_composite(bg, shade.transform((N, N), Image.AFFINE, (1, 0, 0, 0, 1, -14 * k)))

blush = Image.new("RGBA", (N, N))
bd = ImageDraw.Draw(blush)
inner, outer = (249, 168, 212), (255, 255, 255)
for r in range(int(330 * k), 0, -int(4 * k)):
    t = min(1, max(0, (r / k - 60) / 210))
    bd.ellipse([512 * k - r, 512 * k - r, 512 * k + r, 512 * k + r], fill=lerp(inner, outer, t ** 0.7) + (255,))
# Each petal over the one before, as a flower's overlap: its blush, then its edge.
for i in range(5):
    pts = petal(i * 72 + 18, length)
    m = Image.new("L", (N, N))
    ImageDraw.Draw(m).polygon(pts, fill=255)
    bg.paste(blush, (0, 0), m)
    ImageDraw.Draw(bg).line(pts + [pts[0]], fill=(221, 196, 252, 255), width=int(6 * k), joint="curve")

# The center: a column of stamens, magenta, ringed with golden pollen.
cx = cy = 512 * k
d = ImageDraw.Draw(bg)
# The petals' points meet under the center: a blush disk closes the gaps between them.
disk = Image.new("L", (N, N))
ImageDraw.Draw(disk).ellipse([cx - 92 * k, cy - 92 * k, cx + 92 * k, cy + 92 * k], fill=255)
bg.paste(blush, (0, 0), disk)
d = ImageDraw.Draw(bg)
d.ellipse([cx - 56 * k, cy - 56 * k, cx + 56 * k, cy + 56 * k], fill=(236, 72, 153, 255))
for i in range(10):
    a = math.radians(i * 36)
    x, y = cx + math.cos(a) * 38 * k, cy + math.sin(a) * 38 * k
    d.ellipse([x - 10 * k, y - 10 * k, x + 10 * k, y + 10 * k], fill=(253, 224, 71, 255))
d.ellipse([cx - 18 * k, cy - 18 * k, cx + 18 * k, cy + 18 * k], fill=(190, 24, 93, 255))

bg.resize((1024, 1024), Image.LANCZOS).save(out)
bg.resize((32, 32), Image.LANCZOS).save("internal/shell/tray.png")
