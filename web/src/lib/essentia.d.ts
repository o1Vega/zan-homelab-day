// essentia.js ships no ESM types; we only use a handful of algorithms.
declare module 'essentia.js' {
  const mod: any
  export default mod
  export const EssentiaWASM: any
  export const Essentia: any
}
