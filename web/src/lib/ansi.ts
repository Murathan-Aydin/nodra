// Séquences ANSI (couleurs, curseur) retirées pour l'affichage en texte brut.
// eslint-disable-next-line no-control-regex
const ANSI = /\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\r(?!\n)/g

export function stripAnsi(s: string): string {
  return s.replace(ANSI, "")
}
