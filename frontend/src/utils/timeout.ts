// The cancellation function always targets the currently scheduled segment,
// including after the first day has elapsed or the browser delayed a callback.
export function setSafeTimeout(
  callback: () => void,
  delay: number
): () => void {
  const deadline = Date.now() + Math.max(0, Number.isFinite(delay) ? delay : 0);
  let canceled = false;
  let timer: number;
  const schedule = () => {
    if (canceled) return;
    const remaining = Math.max(0, deadline - Date.now());
    timer = window.setTimeout(
      () => {
        if (canceled) return;
        if (Date.now() >= deadline) {
          canceled = true;
          callback();
        } else {
          schedule();
        }
      },
      Math.min(remaining, 86_400_000)
    );
  };
  schedule();
  return () => {
    canceled = true;
    window.clearTimeout(timer);
  };
}
