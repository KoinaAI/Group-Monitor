import { twMerge } from 'tailwind-merge'

// Minimal clsx-style joiner backed by tailwind-merge (clsx isn't a dependency).
// Accepts strings / arrays / falsy values, flattens, then dedupes conflicting
// Tailwind utilities so later classes win (e.g. cn('p-2', cond && 'p-4')).
type ClassValue = string | number | null | false | undefined | ClassValue[]

export function cn(...inputs: ClassValue[]): string {
  const parts: string[] = []
  const walk = (v: ClassValue) => {
    if (v === null || v === undefined || v === false || v === '') return
    if (Array.isArray(v)) v.forEach(walk)
    else parts.push(String(v))
  }
  inputs.forEach(walk)
  return twMerge(parts.join(' '))
}
