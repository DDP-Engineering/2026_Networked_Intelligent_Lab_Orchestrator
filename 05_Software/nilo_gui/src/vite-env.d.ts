/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** `live` to call a real Bench Core backend, `mock` for the contract mock. */
  readonly VITE_NILO_API_MODE?: string;
  /** Absolute base URL of the Bench Core development endpoint. */
  readonly VITE_NILO_API_BASE_URL?: string;
  /** Scenario the contract mock answers with, when the mode is `mock`. */
  readonly VITE_NILO_MOCK_SCENARIO?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
