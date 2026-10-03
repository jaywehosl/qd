let until = 0;

export function expectRestart(ms = 30000): void {
  until = Date.now() + ms;
}

export function restarting(): boolean {
  return Date.now() < until;
}
