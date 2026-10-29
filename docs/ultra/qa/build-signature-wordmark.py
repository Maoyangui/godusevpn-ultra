"""Design-time outline generation; no fonts or shaping libraries ship in the app.

Shape only the literal lowercase word 'ultra' from the locally installed Gabriola font.
The final UI uses SVG geometry, so the VPS does not need a matching font.
Run with fontTools and uharfbuzz available to this Python process.
"""
from pathlib import Path
import json
import re
import sys

from fontTools.ttLib import TTFont
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
import uharfbuzz as hb

root = Path(__file__).resolve().parents[3]
font_path = Path(sys.argv[1])
font_bytes = font_path.read_bytes()
tt = TTFont(font_path)
glyphs = tt.getGlyphSet()
order = tt.getGlyphOrder()
face = hb.Face(font_bytes)
font = hb.Font(face)
font.scale = (face.upem, face.upem)
hb.ot_font_set_funcs(font)
buffer = hb.Buffer()
buffer.add_str("ultra")
buffer.guess_segment_properties()
hb.shape(font, buffer, {"kern": True, "liga": True, "calt": True})
positions = []
x = y = 0
bounds = BoundsPen(glyphs)
for info, pos in zip(buffer.glyph_infos, buffer.glyph_positions):
    name = order[info.codepoint]
    ox, oy = x + pos.x_offset, y + pos.y_offset
    positions.append((name, ox, oy))
    glyphs[name].draw(TransformPen(bounds, (1, 0, 0, 1, ox, oy)))
    x += pos.x_advance
    y += pos.y_advance
x0, y0, x1, y1 = bounds.bounds
# The wordmark is deliberately large enough to read at the 80 px onboarding
# mark.  It overlaps the upper-right of the original glyph by a small amount,
# while remaining entirely inside the 400 px square and keeping the original
# glyph's optical center unchanged.
scale = 190 / (x1 - x0)
width, height = (x1 - x0) * scale, (y1 - y0) * scale
left, top = 194, 64
pen = SVGPathPen(glyphs)
for name, ox, oy in positions:
    transform = (scale, 0, 0, -scale, left + (ox - x0) * scale, top + (y1 - oy) * scale)
    glyphs[name].draw(TransformPen(pen, transform))
# Font contours remain smooth quadratic/cubic geometry; no raster tracing.
outline = re.sub(r"-?\d+\.\d+", lambda m: f"{float(m[0]):.3f}".rstrip('0').rstrip('.'), pen.getCommands())
# A light outline was too thin after the 400 px artwork was rendered at the
# 80 px UI size.  This stroke keeps the Gabriola contours but gives the
# lowercase signature a deliberate, bold ink weight without an outer badge.
wordmark = f'<path class="ultra-signature" fill="url(#g2u)" stroke="url(#g2u)" stroke-width="8" stroke-linejoin="round" paint-order="stroke fill" d="{outline}"/>'
app = root / 'web/dist/app.js'
source = app.read_text(encoding='utf-8')
pattern = r'    <span class="ultra-logo" role="img".*?(?:</span></span>|</svg></span>)'
replacement = '''    <span class="ultra-logo" role="img" aria-label="佛跳墙 Ultra" title="Ultra 版本">
    <svg class="logo" viewBox="0 0 400 400" aria-hidden="true"><defs><linearGradient id="g2" gradientUnits="userSpaceOnUse" x1="70" y1="70" x2="310" y2="270"><stop offset="0" stop-color="#6a44f2"/><stop offset=".55" stop-color="#2f8bff"/><stop offset="1" stop-color="#18e3e8"/></linearGradient><linearGradient id="g2u" x1="0" y1="0" x2="1" y2="0"><stop class="ultra-signature-start" offset="0"/><stop class="ultra-signature-end" offset="1"/></linearGradient></defs><g transform="translate(-9 31)"><g fill="none" stroke="url(#g2)" stroke-width="44" stroke-linecap="round" stroke-linejoin="round"><path d="M86 248V84l146 124v52"/><path d="M332 78l-88 96"/></g><path fill="url(#g2)" d="M118 192l52 30-52 30z"/></g>''' + wordmark + '''</svg></span>'''
source, count = re.subn(pattern, lambda _: replacement, source, count=1, flags=re.S)
if count != 1:
    raise RuntimeError('expected one existing onboarding brand block')
app.write_text(source, encoding='utf-8', newline='\n')
# Keep the distributable square mark in lockstep with the inline onboarding
# mark.  The standalone asset is used by the README and release documentation.
asset = root / 'docs/ultra/assets/ultra-logo-square.svg'
asset_source = asset.read_text(encoding='utf-8')
asset_source, asset_count = re.subn(r'<path class="ultra-signature".*?/>', wordmark, asset_source, count=1, flags=re.S)
if asset_count != 1:
    raise RuntimeError('expected one standalone signature path')
asset.write_text(asset_source, encoding='utf-8', newline='\n')
print(json.dumps({"text": "ultra", "font": "Gabriola", "weight": "bold-outline", "shaping": "HarfBuzz kern/liga/calt", "glyphCount": len(positions), "wordmarkBounds": [round(left,3), top, round(width,3), round(height,3)], "canvas": [400,400], "mainGlyphCenter": [200,200], "pathCharacters": len(outline)}))
