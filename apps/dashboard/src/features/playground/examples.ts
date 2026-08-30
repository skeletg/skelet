// Seed tabs shown the first time an admin opens the playground - GraphiQL
// only applies `defaultTabs` when no persisted tabs exist in localStorage,
// so edits/new tabs are preserved across sessions
//
// Each example uses a named operation so GraphiQL picks up a tab title
// from the operation name

export type ExampleTab = {
  query: string
  variables?: string
  headers?: string
}

export const exampleTabs: ExampleTab[] = [
  {
    query: /* GraphQL */ `# Who am I
# Returns the signed-in admin with their roles and effective permissions.
query Me {
  me {
    id
    email
    firstName
    lastName
    isActive
    isSuperuser
    roles {
      slug
      name
    }
    permissions {
      slug
      name
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# List admins
# Every admin + the roles they belong to.
query ListAdmins {
  admins {
    id
    email
    firstName
    lastName
    isActive
    isSuperuser
    roles {
      slug
      name
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# List roles
# Role catalog with member counts and granted permissions.
query ListRoles {
  roles {
    id
    slug
    name
    description
    memberCount
    permissions {
      slug
      name
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# List customers
query ListCustomers {
  customers {
    id
    email
    firstName
    lastName
    isActive
    isEmailVerified
    group {
      slug
      name
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# List orders
# Includes the status + payment-status snapshot for each row.
query ListOrders {
  orders {
    id
    number
    status
    paymentStatus
    total
    currencyCode
    createdAt
    shippingAddress {
      firstName
      lastName
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# Order detail
# Replace the id variable with a real order id from ListOrders.
query OrderDetail($id: ID!) {
  order(id: $id) {
    number
    status
    paymentStatus
    total
    currencyCode
    lines {
      productName
      productSku
      quantity
      unitPrice
      totalPrice
    }
    events {
      type
      message
      createdAt
    }
  }
}
`,
    variables: JSON.stringify({ id: 'REPLACE_WITH_ORDER_ID' }, null, 2),
  },
  {
    query: /* GraphQL */ `# Configuration tree
# Registry-ordered sections → groups → fields.
query ConfigSections {
  configSections {
    slug
    label
    groups {
      slug
      label
      fields {
        path
        label
        type
        value
        isDefault
        public
      }
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# Set a config value
# Pass the dotted path and a string value - the backend normalises for the
# registered type (int/bool/json round-trip through their parse/marshal pair).
mutation SetConfigValue($path: String!, $value: String!) {
  setConfigValue(path: $path, value: $value) {
    path
    value
    isDefault
    updatedAt
  }
}
`,
    variables: JSON.stringify(
      { path: 'general/store_information/name', value: 'Skelet Demo' },
      null,
      2,
    ),
  },
  {
    query: /* GraphQL */ `# List admin actions
# Registered actions with their typed arguments - powers the Tools page.
query AdminActions {
  adminActions {
    slug
    name
    description
    group
    permissionRequired
    arguments {
      name
      type
      required
      default
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# Run an admin action
# Swap in any registered slug from AdminActions.
mutation RunAction($slug: String!, $args: JSON!) {
  runAdminAction(slug: $slug, args: $args) {
    success
    output
    data
    durationMs
  }
}
`,
    variables: JSON.stringify({ slug: 'config.orphans', args: {} }, null, 2),
  },
  {
    query: /* GraphQL */ `# List integrations
# REST-facing API tokens.
query ListIntegrations {
  integrations {
    id
    name
    description
    isActive
    expiresAt
    permissions {
      slug
      name
    }
  }
}
`,
  },
  {
    query: /* GraphQL */ `# List channels
# Read channel definitions, currency, and subsidiary metadata.
query Channels {
  channels {
    id
    slug
    name
    defaultLocale
    locales
    isActive
    currencyCode
    publicUrl
    subsidiaryLegalName
  }
}
`,
  },
  {
    query: /* GraphQL */ `# Create a channel
# Multi-store: each view has its own currency and subsidiary fields.
# locales defaults to [defaultLocale] when omitted.
mutation CreateChannel($input: CreateChannelInput!) {
  createChannel(input: $input) {
    id
    slug
    name
    defaultLocale
    locales
    isActive
    currencyCode
    publicUrl
  }
}
`,
    variables: JSON.stringify(
      {
        input: {
          slug: 'wheels',
          name: 'Wheels channel',
          defaultLocale: 'en',
          currencyCode: 'USD',
          publicUrl: 'https://wheels.example.com',
        },
      },
      null,
      2,
    ),
  },
  {
    query: /* GraphQL */ `# Set a per-store product listing
# Pass null for price to inherit defaultPrice (only valid when currencies match).
mutation SetProductListing($input: SetProductListingInput!) {
  setProductListing(input: $input) {
    id
    listings {
      isPublished
      price
      effectivePrice
      effectiveCurrency
      channel { slug name currencyCode }
    }
  }
}
`,
    variables: JSON.stringify(
      {
        input: {
          productId: 'REPLACE_WITH_PRODUCT_ID',
          channelId: 'REPLACE_WITH_CHANNEL_ID',
          isPublished: true,
          price: 1999,
        },
      },
      null,
      2,
    ),
  },
]

// The storefront surface resolves its channel from a header, so a seed tab that just runs is worth
// more than another example - without it the first query on that surface fails on a missing channel
export const frontExampleTabs: ExampleTab[] = [
  {
    query: /* GraphQL */ `# Storefront surface
# Most queries resolve a channel from the X-Channel-Id header below - fill it
# in with an active channel id, then browse the Operations panel for the rest.
query FrontChannelCheck {
  __typename
}
`,
    headers: JSON.stringify({ 'X-Channel-Id': 'REPLACE_WITH_CHANNEL_ID' }, null, 2),
  },
]
