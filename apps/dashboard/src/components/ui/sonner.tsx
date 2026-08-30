import { Toaster as Sonner, type ToasterProps } from 'sonner'
import { useTheme } from '@/lib/theme'

// Theme-following toast outlet - the appearance mode maps onto sonner's, and surfaces use the
// app's CSS variables so custom palettes skin toasts like the rest of the chrome
export function Toaster(props: ToasterProps) {
  const { appearance, custom } = useTheme()
  // custom carries its own base, so its switch decides the mode for that palette
  const mode: ToasterProps['theme'] =
    appearance.palette === 'custom' ? (custom.dark ? 'dark' : 'light') : appearance.mode
  return (
    <Sonner
      theme={mode}
      className="toaster group"
      toastOptions={{
        classNames: {
          toast:
            'group toast group-[.toaster]:bg-background group-[.toaster]:text-foreground group-[.toaster]:border-border group-[.toaster]:shadow-lg',
          description: 'group-[.toast]:text-muted-foreground',
          actionButton: 'group-[.toast]:bg-primary group-[.toast]:text-primary-foreground',
          cancelButton: 'group-[.toast]:bg-muted group-[.toast]:text-muted-foreground',
        },
      }}
      {...props}
    />
  )
}
