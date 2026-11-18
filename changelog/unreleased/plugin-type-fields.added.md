- **Plugins put their own data on core's records.** A plugin's SDL can now extend core's own types - a warranty on
  `Product`, an ERP status on `Order` - on both APIs. Such a field is batched, so the plugin answers once per list
  of parents rather than once per row, and nullable, so a channel where the plugin is off answers null. Admin fields
  on core types are gated in the plugin, and the authz tests follow them there as they do root fields.
- **Plugin data leaves with the customer's.** `RegisterCustomerData` adds a plugin's section to a customer's data
  export and scrubs it when the customer is erased.
