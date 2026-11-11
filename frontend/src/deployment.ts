export function isPublicDemo(): boolean {
  return import.meta.env.VITE_PUBLIC_DEMO === 'true'
}
