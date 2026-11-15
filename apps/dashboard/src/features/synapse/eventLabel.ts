// Split from ActivityProfile so that file exports only components - fast refresh needs it

const EVENT_LABELS: Record<string, string> = {
  // Web (Lens) + email (Pulse)
  page_view: 'Page view',
  filter_items: 'Filtered products',
  search: 'Searched',
  custom: 'Event',
  identify: 'Identified',
  email_sent: 'Email sent',
  email_opened: 'Email opened',
  email_clicked: 'Email clicked',
  conversion: 'Conversion',
  // Commerce
  purchase: 'Order placed',
  refund: 'Refunded',
  order_shipped: 'Order shipped',
  giftcard_redeemed: 'Gift card redeemed',
  add_to_cart: 'Added to cart',
  cart_updated: 'Cart updated',
  remove_from_cart: 'Removed from cart',
  add_shipping_info: 'Chose shipping',
  add_payment_info: 'Chose payment',
  add_to_wishlist: 'Added to wishlist',
  remove_from_wishlist: 'Removed from wishlist',
  generate_lead: 'Signed up to a form',
  cart_cleared: 'Cart cleared',
  cart_merged: 'Cart merged',
  review_submitted: 'Review submitted',
  return_requested: 'Return requested',
  return_approved: 'Return approved',
  return_rejected: 'Return rejected',
  return_received: 'Return received',
  subscription_invoiced: 'Subscription invoiced',
  download_accessed: 'Download accessed',
  // AI
  assistant_turn: 'Chatted with the shopping assistant',
  assisted_order: 'Ordered with AI help',
  // Account
  sign_up: 'Registered',
  email_verified: 'Email verified',
  account_updated: 'Account updated',
  profile_updated: 'Profile updated',
  password_changed: 'Password changed',
  address_changed: 'Address changed',
  password_reset_requested: 'Password reset requested',
  login: 'Signed in',
  logout: 'Signed out',
}

// eventLabel maps an event_name to a human label, with a smart fallback for the
// dynamic families (order_<status>, order_payment_<status>, subscription_<verb>)
export const eventLabel = (n: string): string => {
  if (EVENT_LABELS[n]) return EVENT_LABELS[n]
  if (n.startsWith('order_payment_')) return 'Payment ' + n.slice('order_payment_'.length).replace(/_/g, ' ')
  if (n.startsWith('order_')) return 'Order ' + n.slice('order_'.length).replace(/_/g, ' ')
  if (n.startsWith('subscription_')) return 'Subscription ' + n.slice('subscription_'.length).replace(/_/g, ' ')
  return n.replace(/_/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
}
