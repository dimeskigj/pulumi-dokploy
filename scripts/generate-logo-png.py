#!/usr/bin/env python3
"""Render the repository's small, static logo SVG without external tools."""

import re
import struct
import sys
import zlib
from pathlib import Path
from xml.etree import ElementTree

WIDTH = HEIGHT = 175
VIEWBOX = (0.0, 0.0, 457.0, 472.0)
SAMPLES = 4
MAX_IMAGE_PIXELS = 16_777_216
NUMBER = r"[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?"


def parse_logo(path: Path) -> tuple[list[tuple[float, float]], tuple[int, int, int]]:
    root = ElementTree.fromstring(path.read_bytes())
    if root.tag.rsplit("}", 1)[-1] != "svg" or root.attrib.get("viewBox") != "0 0 457 472":
        raise ValueError("logo SVG must use the canonical 457 by 472 viewBox")
    paths = [node for node in root.iter() if node.tag.rsplit("}", 1)[-1] == "path"]
    if len(paths) != 1 or paths[0].attrib.keys() != {"fill", "d"}:
        raise ValueError("logo SVG must contain exactly one plain filled path")
    color = paths[0].attrib["fill"]
    if not re.fullmatch(r"#[0-9a-fA-F]{6}", color):
        raise ValueError("logo path fill must be an RGB hex color")
    tokens = re.findall(rf"[A-Za-z]|{NUMBER}", paths[0].attrib["d"])
    if not tokens or tokens[0] != "M" or tokens[-1] != "z":
        raise ValueError("logo path must start with move and end with close")
    if any(token not in {"M", "c", "l", "z"} for token in tokens if token.isalpha()):
        raise ValueError("logo path contains an unsupported command")
    cursor = 0
    points = []
    command = None
    x = y = 0.0
    while cursor < len(tokens):
        if tokens[cursor].isalpha():
            command = tokens[cursor]
            cursor += 1
        if command == "z":
            break
        if command == "M":
            x, y = float(tokens[cursor]), float(tokens[cursor + 1])
            cursor += 2
            points.append((x, y))
            command = "l"
        elif command == "l":
            x += float(tokens[cursor])
            y += float(tokens[cursor + 1])
            cursor += 2
            points.append((x, y))
        elif command == "c":
            controls = [float(value) for value in tokens[cursor : cursor + 6]]
            if len(controls) != 6:
                raise ValueError("logo path has incomplete cubic geometry")
            cursor += 6
            c1 = (x + controls[0], y + controls[1])
            c2 = (x + controls[2], y + controls[3])
            end = (x + controls[4], y + controls[5])
            start = (x, y)
            for step in range(1, 17):
                t = step / 16
                inverse = 1 - t
                points.append(tuple(
                    inverse**3 * start[index]
                    + 3 * inverse**2 * t * c1[index]
                    + 3 * inverse * t**2 * c2[index]
                    + t**3 * end[index]
                    for index in (0, 1)
                ))
            x, y = end
        else:
            raise ValueError("logo path has invalid geometry")
    return points, tuple(int(color[index : index + 2], 16) for index in (1, 3, 5))


def inside(point: tuple[float, float], polygon: list[tuple[float, float]]) -> bool:
    x, y = point
    result = False
    for index, (x1, y1) in enumerate(polygon):
        x2, y2 = polygon[index - 1]
        if (y1 > y) != (y2 > y) and x < (x2 - x1) * (y - y1) / (y2 - y1) + x1:
            result = not result
    return result


def png_chunk(kind: bytes, data: bytes) -> bytes:
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)


def png_pixel_data(data: bytes) -> tuple[int, int, bytes]:
    signature = b"\x89PNG\r\n\x1a\n"
    if not data.startswith(signature):
        raise ValueError("not a PNG file")
    cursor = 8
    dimensions = None
    compressed = bytearray()
    seen_iend = False
    idat_started = False
    idat_ended = False
    while cursor < len(data):
        if cursor + 12 > len(data):
            raise ValueError("truncated PNG chunk")
        length = struct.unpack(">I", data[cursor : cursor + 4])[0]
        kind = data[cursor + 4 : cursor + 8]
        end = cursor + 12 + length
        if end > len(data):
            raise ValueError("truncated PNG chunk")
        chunk = data[cursor + 8 : cursor + 8 + length]
        actual_crc = struct.unpack(">I", data[cursor + 8 + length : end])[0]
        if actual_crc != zlib.crc32(kind + chunk) & 0xFFFFFFFF:
            raise ValueError("PNG chunk CRC mismatch")
        if len(kind) != 4 or any(not (65 <= value <= 90 or 97 <= value <= 122) for value in kind):
            raise ValueError("PNG chunk type must contain only ASCII letters")
        if not 65 <= kind[2] <= 90:
            raise ValueError("PNG chunk type has a lowercase reserved byte")
        cursor = end
        if dimensions is None and kind != b"IHDR":
            raise ValueError("PNG must start with IHDR")
        if kind not in {b"IHDR", b"IDAT", b"IEND"} and 65 <= kind[0] <= 90:
            raise ValueError("PNG contains an unknown critical chunk")
        if kind == b"IHDR":
            if dimensions is not None or length != 13:
                raise ValueError("PNG must contain exactly one valid IHDR")
            width, height, bit_depth, color_type, compression, filtering, interlace = struct.unpack(
                ">IIBBBBB", chunk
            )
            if width == 0 or height == 0:
                raise ValueError("PNG dimensions must be positive")
            if width * height > MAX_IMAGE_PIXELS:
                raise ValueError("PNG image is too large")
            if (bit_depth, color_type, compression, filtering, interlace) != (8, 6, 0, 0, 0):
                raise ValueError("logo PNG must be an 8-bit RGBA image without interlacing")
            dimensions = (width, height)
        elif kind == b"IDAT":
            if dimensions is None or idat_ended or seen_iend:
                raise ValueError("invalid PNG IDAT order")
            idat_started = True
            compressed.extend(chunk)
        elif kind == b"IEND":
            if dimensions is None or not idat_started or length != 0:
                raise ValueError("invalid PNG IEND")
            seen_iend = True
            break
        elif idat_started:
            idat_ended = True
    if dimensions is None or not seen_iend or cursor != len(data):
        raise ValueError("PNG must end with IEND")
    width, height = dimensions
    stride = width * 4
    row_size = stride + 1
    expected_length = height * row_size
    try:
        decompressor = zlib.decompressobj()
        filtered = decompressor.decompress(compressed, expected_length + 1)
        if len(filtered) > expected_length or not decompressor.eof or decompressor.unused_data or decompressor.unconsumed_tail:
            raise ValueError("PNG image data has trailing or incomplete compressed data")
    except zlib.error as error:
        raise ValueError("invalid PNG image data") from error
    if len(filtered) != height * row_size:
        raise ValueError("PNG scanline data has the wrong length")
    pixels = bytearray()
    previous = bytes(stride)
    for offset in range(0, len(filtered), row_size):
        filter_type = filtered[offset]
        source = filtered[offset + 1 : offset + row_size]
        row = bytearray(stride)
        for index, value in enumerate(source):
            left = row[index - 4] if index >= 4 else 0
            above = previous[index]
            upper_left = previous[index - 4] if index >= 4 else 0
            if filter_type == 0:
                reconstructed = value
            elif filter_type == 1:
                reconstructed = value + left
            elif filter_type == 2:
                reconstructed = value + above
            elif filter_type == 3:
                reconstructed = value + (left + above) // 2
            elif filter_type == 4:
                prediction = left + above - upper_left
                distances = (abs(prediction - left), abs(prediction - above), abs(prediction - upper_left))
                reconstructed = value + (left if distances[0] <= distances[1] and distances[0] <= distances[2] else above if distances[1] <= distances[2] else upper_left)
            else:
                raise ValueError("PNG uses an unsupported scanline filter")
            row[index] = reconstructed & 255
        pixels.extend(row)
        previous = bytes(row)
    return (*dimensions, bytes(pixels))


def png_pixels_equal(left: bytes, right: bytes) -> bool:
    return png_pixel_data(left) == png_pixel_data(right)


def render(points: list[tuple[float, float]], color: tuple[int, int, int]) -> bytes:
    scale_x = WIDTH / VIEWBOX[2]
    scale_y = HEIGHT / VIEWBOX[3]
    rows = []
    for y in range(HEIGHT):
        row = bytearray([0])
        for x in range(WIDTH):
            covered = 0
            for sy in range(SAMPLES):
                for sx in range(SAMPLES):
                    source = ((x + (sx + 0.5) / SAMPLES) / scale_x,
                              (y + (sy + 0.5) / SAMPLES) / scale_y)
                    covered += inside(source, points)
            alpha = round(255 * covered / (SAMPLES * SAMPLES))
            row.extend((*color, alpha))
        rows.append(row)
    header = struct.pack(">IIBBBBB", WIDTH, HEIGHT, 8, 6, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + png_chunk(b"IHDR", header) + png_chunk(b"IDAT", zlib.compress(b"".join(rows), 9)) + png_chunk(b"IEND", b"")


def main() -> int:
    if len(sys.argv) == 4 and sys.argv[1] == "--check-pixels":
        reference = sys.stdin.buffer.read() if sys.argv[2] == "-" else Path(sys.argv[2]).read_bytes()
        generated = Path(sys.argv[3]).read_bytes()
        if not png_pixels_equal(reference, generated):
            raise SystemExit("logo pixels differ")
        return 0
    if len(sys.argv) != 3:
        raise SystemExit("usage: generate-logo-png.py SVG OUTPUT | --check-pixels REFERENCE OUTPUT")
    points, color = parse_logo(Path(sys.argv[1]))
    output = Path(sys.argv[2])
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(output.suffix + ".tmp")
    temporary.write_bytes(render(points, color))
    temporary.replace(output)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ElementTree.ParseError, ValueError) as error:
        raise SystemExit(f"logo rendering failed: {error}")
