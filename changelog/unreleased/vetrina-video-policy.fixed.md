- **Videos play in Vetrina CMS pages and product content.** The storefront's own CSP blocked YouTube
  and Vimeo players (there was no `frame-src`) and video files (there was no `media-src`). Embeds now
  always use YouTube's privacy-enhanced `youtube-nocookie.com` host or Vimeo's player, and the CSP
  allows exactly those two players plus same-origin, upload-preview and HTTPS video (HTTP too in
  development). Embed links for any other host are no longer rendered. Who may frame the storefront
  is unchanged.
