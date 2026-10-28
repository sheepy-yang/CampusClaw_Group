export type ViewState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "empty"; message: string }
  | { status: "error"; code: string; message: string; requestId?: string };

export function isReady<T>(state: ViewState<T>): state is { status: "ready"; data: T } {
  return state.status === "ready";
}
