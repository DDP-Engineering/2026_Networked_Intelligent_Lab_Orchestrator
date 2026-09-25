"""Image rendering contracts tested with fixed in-memory frames."""

from PIL import Image

from nilo_camera.processing import render_image, validate_processing


def test_scene_zoom_rotation_and_marker_follow_source_coordinates():
    frame = Image.new("RGB", (100, 100), "black")
    for y in range(100):
        for x in range(100):
            frame.putpixel((x, y), (x * 2, y * 2, 0))
    options = validate_processing(
        {
            "zoom": 2,
            "center_x": 0.5,
            "center_y": 0.5,
            "rotation": 90,
            "overlays": [{"kind": "crosshair", "x": 0.5, "y": 0.5, "color": "#ffffff"}],
        }
    )
    result = render_image(frame, options, {}, 100, 100)
    assert result.size == (100, 100)
    assert result.getpixel((50, 50)) == (255, 255, 255)
    assert result.getpixel((5, 5)) != frame.getpixel((5, 5))


def test_invalid_overlay_cannot_create_unbounded_work():
    from nilo_integration import ExecuteFailure

    for value in (
        {"zoom": 9},
        {"overlays": [{"kind": "text", "text": "x" * 121}]},
        {"overlays": [{}] * 33},
    ):
        try:
            validate_processing(value)
        except ExecuteFailure as error:
            assert error.code == "invalid_parameters"
        else:
            raise AssertionError("invalid processing was accepted")


def test_bundled_unicode_font_honors_text_size():
    frame = Image.new("RGB", (200, 80), "black")
    def rendered(size):
        options = validate_processing({"overlays": [
            {"kind": "text", "x": 0.1, "y": 0.1, "text": "ȘЖ", "size": size}
        ]})
        return render_image(frame, options, {}, 200, 80)

    small = rendered(8)
    large = rendered(32)
    small_pixels = sum(pixel != (0, 0, 0) for pixel in small.getdata())
    large_pixels = sum(pixel != (0, 0, 0) for pixel in large.getdata())
    assert large_pixels > small_pixels * 3
