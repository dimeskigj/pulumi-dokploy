import importlib.util
import unittest
import struct
import zlib
from pathlib import Path


SCRIPT = Path(__file__).with_name("generate-logo-png.py")
SPEC = importlib.util.spec_from_file_location("generate_logo_png", SCRIPT)
assert SPEC and SPEC.loader
logo = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(logo)


class LogoPngComparisonTests(unittest.TestCase):
    def test_compression_changes_are_ignored_but_pixel_changes_are_detected(self):
        original = logo.render(*logo.parse_logo(Path("website/public/logo.svg")))
        raw = filtered_data(original)
        recompressed = recompress_idat(original, raw, 1)
        changed_pixels = bytearray(raw)
        changed_pixels[-1] ^= 1
        changed = recompress_idat(original, bytes(changed_pixels), 9)

        self.assertNotEqual(original, recompressed)
        self.assertTrue(logo.png_pixels_equal(original, recompressed))
        self.assertFalse(logo.png_pixels_equal(original, changed))

    def test_rejects_corrupt_png_structure(self):
        valid = make_png(2, 1, [b"\x00\x01\x02\x03\x04\x05\x06\x07\x08"])
        cases = {
            "bad CRC": valid[:29] + bytes([valid[29] ^ 1]) + valid[30:],
            "truncated chunk": valid[:-1],
            "trailing data": valid + b"trailing",
            "IDAT before IHDR": b"\x89PNG\r\n\x1a\n" + logo.png_chunk(b"IDAT", b"") + valid[8:],
            "junk after compressed IDAT": make_png(2, 1, [b"\x00\x01\x02\x03\x04\x05\x06\x07\x08"], b"junk"),
            "unknown critical chunk": valid[:33] + logo.png_chunk(b"ABCD", b"") + valid[33:],
            "invalid chunk name": valid[:33] + logo.png_chunk(b"ab1d", b"") + valid[33:],
            "lowercase reserved byte": valid[:33] + logo.png_chunk(b"abcD", b"") + valid[33:],
            "wrong dimensions": make_png(3, 1, [b"\x00\x01\x02\x03\x04\x05\x06\x07\x08"]),
            "wrong scanline length": make_png(2, 1, [b"\x00\x01"]),
            "decompression exceeds scanline bound": make_png(1, 1, [b"\x00" + b"\x00" * 4 + b"x" * 1_000_000]),
        }
        for name, data in cases.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                logo.png_pixel_data(data)

    def test_compares_decoded_pixels_across_filter_modes(self):
        pixels = bytes(range(16))
        unfiltered = make_png(2, 2, [b"\x00" + pixels[:8], b"\x00" + pixels[8:]])
        filtered_rows = []
        for row in (pixels[:8], pixels[8:]):
            filtered_rows.append(b"\x01" + bytes((value - (row[index - 4] if index >= 4 else 0)) & 255 for index, value in enumerate(row)))
        filtered = make_png(2, 2, filtered_rows)

        self.assertTrue(logo.png_pixels_equal(unfiltered, filtered))

    def test_compares_decoded_pixels_for_up_average_and_paeth_filters(self):
        pixels = bytes(range(24))
        filtered = make_filtered_png(2, 3, pixels, [2, 3, 4])
        unfiltered = make_png(2, 3, [b"\x00" + pixels[:8], b"\x00" + pixels[8:16], b"\x00" + pixels[16:]])

        self.assertTrue(logo.png_pixels_equal(unfiltered, filtered))


def recompress_idat(data, raw, level):
    cursor = 8
    output = bytearray(data[:8])
    replaced = False
    while cursor < len(data):
        length = struct.unpack(">I", data[cursor : cursor + 4])[0]
        kind = data[cursor + 4 : cursor + 8]
        chunk = data[cursor + 8 : cursor + 8 + length]
        cursor += length + 12
        if kind == b"IDAT":
            if not replaced:
                output.extend(logo.png_chunk(kind, zlib.compress(raw, level)))
                replaced = True
        else:
            output.extend(logo.png_chunk(kind, chunk))
    return bytes(output)


def filtered_data(data):
    cursor = 8
    compressed = bytearray()
    while cursor < len(data):
        length = struct.unpack(">I", data[cursor : cursor + 4])[0]
        kind = data[cursor + 4 : cursor + 8]
        if kind == b"IDAT":
            compressed.extend(data[cursor + 8 : cursor + 8 + length])
        cursor += length + 12
    return zlib.decompress(compressed)


def make_png(width, height, rows, idat_suffix=b""):
    header = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + logo.png_chunk(b"IHDR", header) + logo.png_chunk(
        b"IDAT", zlib.compress(b"".join(rows)) + idat_suffix
    ) + logo.png_chunk(b"IEND", b"")


def make_filtered_png(width, height, pixels, filters):
    rows = []
    previous = bytes(width * 4)
    for row_number, filter_type in enumerate(filters):
        row = pixels[row_number * width * 4 : (row_number + 1) * width * 4]
        encoded = bytearray()
        for index, value in enumerate(row):
            left = row[index - 4] if index >= 4 else 0
            above = previous[index]
            upper_left = previous[index - 4] if index >= 4 else 0
            if filter_type == 2:
                prediction = above
            elif filter_type == 3:
                prediction = (left + above) // 2
            else:
                prediction = paeth(left, above, upper_left)
            encoded.append((value - prediction) & 255)
        rows.append(bytes([filter_type]) + encoded)
        previous = row
    return make_png(width, height, rows)


def paeth(left, above, upper_left):
    estimate = left + above - upper_left
    distances = (abs(estimate - left), abs(estimate - above), abs(estimate - upper_left))
    return left if distances[0] <= distances[1] and distances[0] <= distances[2] else above if distances[1] <= distances[2] else upper_left


if __name__ == "__main__":
    unittest.main()
