import type { AnchorHTMLAttributes } from "react";

// LocaleLink wraps next/link and the App Router hooks, none of which exist outside a Next render. Tests that only care
// that a link is there swap it for a plain anchor: vi.mock("@/app/components/LocaleLink", () => localeLinkMock)
export const localeLinkMock = {
  Link: ({ href, children, ...rest }: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
  useChannelId: () => "test-channel",
  usePathname: () => "/",
  useRouter: () => ({ push: () => {}, replace: () => {}, refresh: () => {} }),
};
