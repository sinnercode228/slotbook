/** Up to two capital initials of a name, for avatars and the brand mark. */
export function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter((word) => /^\p{L}/u.test(word))
    .slice(0, 2)
    .map((word) => word[0]!.toUpperCase())
    .join('');
}
