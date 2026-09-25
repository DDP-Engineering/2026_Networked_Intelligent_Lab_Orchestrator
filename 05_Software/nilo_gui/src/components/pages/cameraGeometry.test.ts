import { expect, it } from 'vitest';
import { toImagePoint, toScenePoint } from './cameraGeometry';

it('maps zoomed and flipped viewer clicks back to image coordinates', () => {
  expect(
    toImagePoint(
      0.75,
      0.5,
      {
        zoom: 2,
        panX: 0.5,
        panY: 0.5,
        rotation: 0,
        flipHorizontal: false,
        flipVertical: false,
      },
      640,
      480,
    ),
  ).toEqual({ x: 0.625, y: 0.5 });
  expect(
    toImagePoint(
      0.25,
      0.5,
      {
        zoom: 1,
        panX: 0.5,
        panY: 0.5,
        rotation: 0,
        flipHorizontal: true,
        flipVertical: false,
      },
      640,
      480,
    ),
  ).toEqual({ x: 0.75, y: 0.5 });
});

it('maps a composed output marker to the mounted camera scene', () => {
  expect(
    toScenePoint(0.75, 0.5, {
      zoom: 2,
      centerX: 0.5,
      centerY: 0.5,
      rotation: 0,
      flipHorizontal: false,
      flipVertical: false,
    }),
  ).toEqual({ x: 0.625, y: 0.5 });
  expect(
    toScenePoint(0.75, 0.5, {
      zoom: 2,
      centerX: 0.5,
      centerY: 0.5,
      rotation: 90,
      flipHorizontal: false,
      flipVertical: false,
    }),
  ).toEqual({ x: 0.5, y: 0.625 });
});
