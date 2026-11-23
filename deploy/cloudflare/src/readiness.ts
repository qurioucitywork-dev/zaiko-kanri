// Keep readiness scoped to one live process. A failed startup is retryable, but
// application requests themselves are never replayed by this gate.
export class ContainerReadiness {
  private ready = false;
  private starting?: Promise<void>;

  ensure(running: boolean, initialize: () => Promise<void>): Promise<void> {
    if (this.ready && running) return Promise.resolve();
    this.ready = false;
    this.starting ??= Promise.resolve().then(initialize).then(() => {
      this.ready = true;
    }).finally(() => {
      this.starting = undefined;
    });
    return this.starting;
  }

  invalidate(): void {
    this.ready = false;
  }
}
