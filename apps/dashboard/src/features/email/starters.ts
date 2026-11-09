import type { EmailBlock } from './blocks'

export type StarterTemplate = {
  id: string
  name: string
  description: string
  group: 'marketing' | 'automation' | 'order'
  blocks: EmailBlock[]
}

export const STARTER_GROUPS: Array<{ id: StarterTemplate['group']; label: string }> = [
  { id: 'marketing', label: 'Marketing' },
  { id: 'automation', label: 'Automations' },
  { id: 'order', label: 'Order emails' },
]

const center = { align: 'center' as const }
const logo: EmailBlock = { type: 'image', url: '{{store.logoUrl}}', alt: '{{store.name}}', style: { align: 'center', width: 30 } }
const footer: EmailBlock = {
  type: 'store_footer',
  content: '{{store.name}}\nYou get these emails because you signed up at our store.',
  label: 'Unsubscribe',
}
const rich = (html: string, style?: EmailBlock['style']): EmailBlock => ({ type: 'text', format: 'rich', content: html, ...(style ? { style } : {}) })

export const STARTER_TEMPLATES: StarterTemplate[] = [
  {
    id: 'welcome',
    name: 'Welcome',
    description: 'Greet a new customer and send them to the shop.',
    group: 'marketing',
    blocks: [
      logo,
      { type: 'heading', text: 'Welcome to {{store.name}}', style: center },
      rich('<p>Hi {{customer.firstName}},</p><p>Thanks for joining us. Here is what to expect: new arrivals first, members-only offers, and help whenever you need it.</p>'),
      { type: 'button', label: 'Start shopping', url: '{{storefront.url}}', style: center },
      footer,
    ],
  },
  {
    id: 'announcement',
    name: 'Announcement',
    description: 'One piece of news with a link to read more.',
    group: 'marketing',
    blocks: [
      { type: 'heading', text: 'Something new at {{store.name}}' },
      rich('<p>Tell your customers what changed and why it matters to them.</p>'),
      { type: 'button', label: 'Read more', url: '{{storefront.url}}' },
      rich('<p>Questions? Just reply to this email.</p>', { fontSize: 13 }),
      footer,
    ],
  },
  {
    id: 'promo',
    name: 'Promotion',
    description: 'A sale or a collection, with a big image and a call to action.',
    group: 'marketing',
    blocks: [
      { type: 'image', url: '', alt: 'The collection' },
      { type: 'heading', text: 'The new collection is here', style: center },
      rich('<p>Hand-picked pieces for the season. <strong>Free shipping</strong> on every order this week.</p>', center),
      { type: 'button', label: 'Shop the collection', url: '{{storefront.url}}', style: center },
      footer,
    ],
  },
  {
    id: 'newsletter',
    name: 'Newsletter',
    description: 'A menu, an intro and two stories with pictures.',
    group: 'marketing',
    blocks: [
      {
        type: 'menu',
        links: [
          { label: 'Shop', url: '{{storefront.url}}' },
          { label: 'New in', url: '{{storefront.url}}/new' },
          { label: 'Journal', url: '{{storefront.url}}/blog' },
        ],
      },
      { type: 'heading', text: 'This month at {{store.name}}' },
      rich('<p>Hi {{customer.firstName}}, here is what we have been working on.</p>'),
      { type: 'image_text', image: '', alt: '', text: 'The first story', content: 'A few lines on what is new and why it is worth a look.', label: 'Read more', url: '{{storefront.url}}', side: 'left' },
      { type: 'image_text', image: '', alt: '', text: 'The second story', content: 'Behind the scenes, a guide, or a customer favourite.', label: 'Read more', url: '{{storefront.url}}', side: 'right' },
      { type: 'divider' },
      footer,
    ],
  },
  {
    id: 'new_arrivals',
    name: 'New arrivals',
    description: 'A hero image and a grid of the newest products.',
    group: 'marketing',
    blocks: [
      { type: 'image', url: '', alt: 'New arrivals' },
      { type: 'heading', text: 'Just landed', style: center },
      rich('<p>Fresh from the workshop - pick yours before they are gone.</p>', center),
      { type: 'products', products: { mode: 'skus', skus: [], limit: 3, columns: 3 } },
      { type: 'button', label: 'See everything new', url: '{{storefront.url}}/new', style: center },
      footer,
    ],
  },
  {
    id: 'flash_sale',
    name: 'Flash sale',
    description: 'A short sale with a deadline and a code.',
    group: 'marketing',
    blocks: [
      { type: 'heading', text: '48 hours only', style: { align: 'center', fontSize: 28 } },
      { type: 'countdown', label: 'Ends', deadline: 'Sunday at midnight', style: center },
      { type: 'image', url: '', alt: 'The sale' },
      { type: 'coupon', text: '20% off everything', code: 'FLASH20', content: 'Applies at checkout until the sale ends.', label: 'Shop the sale', url: '{{storefront.url}}' },
      footer,
    ],
  },
  {
    id: 'abandoned_cart',
    name: 'Abandoned cart',
    description: 'Bring a shopper back to the cart they left.',
    group: 'automation',
    blocks: [
      logo,
      { type: 'heading', text: 'You left something behind', style: center },
      rich('<p>Hi {{customer.firstName}}, your cart is saved and waiting for you. Finish your order whenever you are ready.</p>', center),
      { type: 'button', label: 'Return to your cart', url: '{{cart.recoveryUrl}}', style: center },
      { type: 'coupon', text: 'Still deciding?', code: 'COMEBACK10', content: '10% off if you finish your order today.' },
      footer,
    ],
  },
  {
    id: 'win_back',
    name: 'We miss you',
    description: 'Win back a customer who has not ordered in a while.',
    group: 'automation',
    blocks: [
      { type: 'heading', text: 'It has been a while, {{customer.firstName}}', style: center },
      rich('<p>A lot has changed since your last visit - new products, new favourites. Here is a little something to welcome you back.</p>', center),
      { type: 'coupon', text: 'Welcome back', code: 'MISSYOU15', content: '15% off your next order.', label: 'See what is new', url: '{{storefront.url}}/new' },
      footer,
    ],
  },
  {
    id: 'thank_you',
    name: 'Thank you',
    description: 'A follow-up after a purchase, with care tips and a review ask.',
    group: 'automation',
    blocks: [
      { type: 'heading', text: 'Thank you, {{customer.firstName}}' },
      rich('<p>We hope you love your order. A couple of things to help you get the most out of it:</p>'),
      { type: 'image_text', image: '', alt: '', text: 'Make it last', content: 'Our care guide covers cleaning, storage and small repairs.', label: 'Read the guide', url: '{{storefront.url}}', side: 'left' },
      rich('<p>Have a minute? A review helps other shoppers - and us.</p>'),
      { type: 'button', label: 'Leave a review', url: '{{storefront.url}}' },
      footer,
    ],
  },
  {
    id: 'order_confirmation',
    name: 'Order confirmation',
    description: 'What was ordered, what it cost, and where it is going.',
    group: 'order',
    blocks: [
      logo,
      { type: 'heading', text: 'Thanks for your order' },
      rich('<p>Hi {{customer.firstName}}, we have received order <strong>{{order.number}}</strong> and are getting it ready.</p>'),
      { type: 'order_items' },
      { type: 'order_totals' },
      { type: 'shipping_address' },
      { type: 'button', label: 'View your order', url: '{{storefront.url}}' },
    ],
  },
  {
    id: 'shipping_update',
    name: 'Shipping update',
    description: 'Tracking for an order that is on its way.',
    group: 'order',
    blocks: [
      logo,
      { type: 'heading', text: 'Your order is on its way' },
      rich('<p>Good news, {{customer.firstName}} - order <strong>{{order.number}}</strong> has shipped.</p>'),
      { type: 'tracking' },
      { type: 'order_items' },
      { type: 'button', label: 'Track your parcel', url: '{{storefront.url}}' },
    ],
  },
]
