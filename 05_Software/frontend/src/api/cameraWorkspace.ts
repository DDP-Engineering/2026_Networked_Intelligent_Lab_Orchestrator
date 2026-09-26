import type { Credential } from './client';
import { CSRF_HEADER_NAME, resourceUrl } from './contract';

export interface CameraSettings {
  width: number;
  height: number;
  format: string;
  fps: number;
  processing: {
    zoom?: number;
    center_x?: number;
    center_y?: number;
    rotation?: number;
    grayscale?: boolean;
    invert?: boolean;
    flip_horizontal?: boolean;
    flip_vertical?: boolean;
    overlays?: Record<string, unknown>[];
  };
  stamps: {
    kind: string;
    text?: string;
    position: string;
    background: string;
    size: number;
    timezone: string;
  }[];
  encoding: string;
  quality: number;
  mode: string;
  interval: number;
  playback_fps: number;
  video_quality: number;
}
export const defaultCameraSettings: CameraSettings = {
  width: 640,
  height: 480,
  format: 'MJPG',
  fps: 15,
  processing: { zoom: 1 },
  stamps: [],
  encoding: 'jpeg',
  quality: 85,
  mode: 'video',
  interval: 5,
  playback_fps: 15,
  video_quality: 23,
};
export interface CameraRetention {
  snapshots: number;
  total_bytes: number;
  video_bytes: number;
  video_seconds: number;
}
export interface CameraWorkspaceState {
  state: string;
  error?: string;
  captured_at?: string;
  expires_at?: string;
  revision: number;
  settings: Partial<CameraSettings>;
  retention: CameraRetention;
  used_bytes: number;
  retention_inventory?: {
    count: number;
    bytes: number;
  };
  recording?: {
    id: string;
    elapsed: number;
    bytes: number;
    frames: number;
    mode: string;
    max_seconds?: number;
    max_bytes?: number;
  } | null;
}
export interface CameraMedia {
  id: string;
  camera_id: string;
  kind: string;
  state: string;
  bytes: number;
  created_at: string;
  metadata: {
    width?: number;
    height?: number;
    duration?: number;
    reason?: string;
    extension?: string;
  };
}

/** Camera media uses the same authenticated origin and CSRF boundary as Bench. */
export function cameraWorkspaceAPI(baseUrl: string, credential: Credential) {
  const url = (path: string) => resourceUrl(baseUrl, path);
  async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (credential.kind === 'bearer')
      headers.Authorization = `Bearer ${credential.token}`;
    else if (method !== 'GET') headers[CSRF_HEADER_NAME] = credential.csrfToken;
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    const response = await fetch(url(path), {
      method,
      headers,
      credentials: 'include',
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(path.endsWith('/stop') ? 35000 : 15000),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      throw new Error(error.error?.code ?? error.code ?? `HTTP ${response.status}`);
    }
    return response.json() as Promise<T>;
  }
  return {
    status: (id: string) =>
      request<CameraWorkspaceState>(
        `/api/v1/cameras/${encodeURIComponent(id)}/workspace`,
      ),
    list: (id: string, offset: number, kind = 'all') =>
      request<{ items: CameraMedia[] }>(
        `/api/v1/cameras/${encodeURIComponent(id)}/media?offset=${offset}&kind=${encodeURIComponent(kind)}`,
      ),
    remove: (id: string) =>
      request(`/api/v1/camera-media/${encodeURIComponent(id)}`, 'DELETE'),
    retention: (id: string, revision: number, limits: CameraRetention) =>
      request(`/api/v1/cameras/${encodeURIComponent(id)}/retention`, 'PUT', {
        revision,
        limits,
      }),
    stop: (id: string, action: 'stop' | 'record_stop') =>
      request(`/api/v1/cameras/${encodeURIComponent(id)}/stop`, 'POST', { action }),
    frame: (id: string) => url(`/api/v1/cameras/${encodeURIComponent(id)}/frame`),
    content: (id: string, option = '') =>
      url(`/api/v1/camera-media/${encodeURIComponent(id)}/content${option}`),
  };
}
