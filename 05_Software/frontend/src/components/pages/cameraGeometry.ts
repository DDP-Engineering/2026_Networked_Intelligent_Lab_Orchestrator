/** Coordinates in the displayed, mounted camera image. */
export interface CameraPoint {
  x: number;
  y: number;
}

export interface CameraViewerTransform {
  zoom: number;
  panX: number;
  panY: number;
  rotation: number;
  flipHorizontal: boolean;
  flipVertical: boolean;
}

export interface CameraComposition {
  zoom: number;
  centerX: number;
  centerY: number;
  rotation: number;
  flipHorizontal: boolean;
  flipVertical: boolean;
}

const bounded = (value: number) =>
  Math.round(Math.min(1, Math.max(0, value)) * 1e6) / 1e6;

/** Undo local CSS transforms so a viewport click identifies an image point. */
export function toImagePoint(
  x: number,
  y: number,
  view: CameraViewerTransform,
  width: number,
  height: number,
): CameraPoint {
  const originX = view.panX * width;
  const originY = view.panY * height;
  const dx = (x * width - originX) / view.zoom;
  const dy = (y * height - originY) / view.zoom;
  const angle = (-view.rotation * Math.PI) / 180;
  let imageX = Math.cos(angle) * dx - Math.sin(angle) * dy;
  let imageY = Math.sin(angle) * dx + Math.cos(angle) * dy;
  if (view.flipHorizontal) imageX = -imageX;
  if (view.flipVertical) imageY = -imageY;
  return {
    x: bounded((originX + imageX) / width),
    y: bounded((originY + imageY) / height),
  };
}

/** Project an image marker into the local transformed viewport. */
export function toViewerPoint(
  point: CameraPoint,
  view: CameraViewerTransform,
  width: number,
  height: number,
): CameraPoint {
  const originX = view.panX * width;
  const originY = view.panY * height;
  let dx = point.x * width - originX;
  let dy = point.y * height - originY;
  if (view.flipHorizontal) dx = -dx;
  if (view.flipVertical) dy = -dy;
  const angle = (view.rotation * Math.PI) / 180;
  const visualX = Math.cos(angle) * dx - Math.sin(angle) * dy;
  const visualY = Math.sin(angle) * dx + Math.cos(angle) * dy;
  return {
    x: (originX + visualX * view.zoom) / width,
    y: (originY + visualY * view.zoom) / height,
  };
}

/** Undo requested crop and transforms for a runner overlay in scene space. */
export function toScenePoint(
  x: number,
  y: number,
  composition: CameraComposition,
): CameraPoint {
  let outputX = composition.flipHorizontal ? 1 - x : x;
  let outputY = composition.flipVertical ? 1 - y : y;
  if (composition.rotation === 90) [outputX, outputY] = [1 - outputY, outputX];
  else if (composition.rotation === 180)
    [outputX, outputY] = [1 - outputX, 1 - outputY];
  else if (composition.rotation === 270) [outputX, outputY] = [outputY, 1 - outputX];
  const visible = 1 / composition.zoom;
  const left = Math.min(1 - visible, Math.max(0, composition.centerX - visible / 2));
  const top = Math.min(1 - visible, Math.max(0, composition.centerY - visible / 2));
  return { x: bounded(left + outputX * visible), y: bounded(top + outputY * visible) };
}
