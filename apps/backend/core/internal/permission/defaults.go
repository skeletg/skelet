package permission

// Declares the core platform permissions - the same set migrations 000002 and 000010 seed
// Declaring them here too is load-bearing: Sync compares the registry to the DB and inserts what's missing, so without this init() anything added via the registry from now on would only live in code

func init() {
	RegisterGroup(Group{Slug: "products", Name: "Products", SortOrder: 1})
	RegisterGroup(Group{Slug: "orders", Name: "Orders", SortOrder: 2})
	RegisterGroup(Group{Slug: "customers", Name: "Customers", SortOrder: 3})
	RegisterGroup(Group{Slug: "admins", Name: "Admins", SortOrder: 4})
	RegisterGroup(Group{Slug: "config", Name: "Configuration", SortOrder: 5})
	RegisterGroup(Group{Slug: "catalog", Name: "Catalog", SortOrder: 6})
	RegisterGroup(Group{Slug: "search", Name: "Search", SortOrder: 7})
	RegisterGroup(Group{Slug: "developer", Name: "Developer Tools", SortOrder: 8})
	RegisterGroup(Group{Slug: "reports", Name: "Reports", SortOrder: 13})

	Register(Definition{Group: "products", Slug: "products.view", Name: "View Products", SortOrder: 1})
	Register(Definition{Group: "products", Slug: "products.create", Name: "Create Products", SortOrder: 2})
	Register(Definition{Group: "products", Slug: "products.update", Name: "Update Products", SortOrder: 3})
	Register(Definition{Group: "products", Slug: "products.delete", Name: "Delete Products", SortOrder: 4})

	Register(Definition{Group: "orders", Slug: "orders.view", Name: "View Orders", SortOrder: 1})
	Register(Definition{Group: "orders", Slug: "orders.update", Name: "Update Orders", SortOrder: 2})
	Register(Definition{Group: "orders", Slug: "orders.refund", Name: "Refund Orders", Description: "Issue refunds and credit memos against any order in scope, including goodwill credits with no return behind them. Money leaves on this permission - grant it separately from orders.update.", SortOrder: 3})

	Register(Definition{Group: "customers", Slug: "customers.view", Name: "View Customers", SortOrder: 1})
	Register(Definition{Group: "customers", Slug: "customers.manage", Name: "Manage Customers", SortOrder: 2})

	Register(Definition{Group: "admins", Slug: "admins.view", Name: "View Admins", SortOrder: 1})
	Register(Definition{Group: "admins", Slug: "admins.manage", Name: "Manage Admins", Description: "Create admins, assign roles and edit permission grants. Effectively lets the holder grant themselves anything else, so treat it as the highest privilege short of superuser.", SortOrder: 2})

	// config.manage gates writes to channel-scoped overrides, config.manage_defaults gates the platform default scope that channels inherit from - split so role assignment can keep channel admins out of the platform baseline
	Register(Definition{Group: "config", Slug: "config.view", Name: "View Configuration", SortOrder: 1})
	Register(Definition{Group: "config", Slug: "config.manage", Name: "Manage Channel Configuration", SortOrder: 2})
	Register(Definition{Group: "config", Slug: "config.manage_defaults", Name: "Manage Default Configuration", Description: "Edit settings at the base scope, which changes them for every channel at once. Distinct from config.manage, which only reaches the selected channel's overrides.", SortOrder: 3})
	Register(Definition{Group: "config", Slug: "config.manage_secrets", Name: "Manage Secret Configuration", Description: "Write or clear secret-typed settings - gateway API keys, webhook signing secrets, SMTP credentials. Separate from config.manage because repointing where money settles should not ride the same grant as changing a date format.", SortOrder: 4})

	Register(Definition{Group: "catalog", Slug: "catalog.view", Name: "View Catalog Configuration", SortOrder: 1})
	Register(Definition{Group: "catalog", Slug: "catalog.manage", Name: "Manage Catalog Configuration", SortOrder: 2})

	Register(Definition{Group: "search", Slug: "search.view", Name: "View Search Status", SortOrder: 1})
	Register(Definition{Group: "search", Slug: "search.manage", Name: "Reindex Search", SortOrder: 2})

	Register(Definition{Group: "developer", Slug: "developer.playground", Name: "Use GraphQL Playground", Description: "Open the GraphQL playground and run introspection against the admin schema. Exposes the full admin API shape, so it belongs with developers rather than operators.", SortOrder: 1})
	Register(Definition{Group: "developer", Slug: "webhooks.view", Name: "View Webhooks", SortOrder: 2})
	Register(Definition{Group: "developer", Slug: "webhooks.manage", Name: "Manage Webhooks", SortOrder: 3})
	Register(Definition{Group: "developer", Slug: "remote_providers.view", Name: "View Remote Providers", SortOrder: 4})
	Register(Definition{Group: "developer", Slug: "remote_providers.manage", Name: "Manage Remote Providers", SortOrder: 5})

	// Read-only aggregate sales and analytics, no per-order PII, so it's a dedicated slug rather than a reuse of orders.view
	Register(Definition{Group: "reports", Slug: "reports.view", Name: "View Reports", SortOrder: 1})

	// The Signals capabilities. Declared here because the role editor and the grants are core's - the
	// suite only reads the slug off a token, the way it reads a channel id or an event name
	RegisterGroup(Group{Slug: "experiments", Name: "Experiments", SortOrder: 14}) // own group, so a role can manage optimization without broader marketing/settings access
	Register(Definition{Group: "experiments", Slug: "experiments.view", Name: "View Experiments", SortOrder: 1})
	Register(Definition{Group: "experiments", Slug: "experiments.manage", Name: "Manage Experiments", SortOrder: 2})

	RegisterGroup(Group{Slug: "lens", Name: "Web Analytics", SortOrder: 16})                         // own capability, so a role can get analytics access without broader settings/customer reach
	Register(Definition{Group: "lens", Slug: "lens.view", Name: "View Web Analytics", SortOrder: 1}) // no .manage twin - Lens is query-only, and the one that existed gated nothing; add it back with the write path it guards

	// Pulse - campaigns, automation flows and the email builder they render from, each its own group so a
	// role can be granted one without the others
	RegisterGroup(Group{Slug: "campaigns", Name: "Email Campaigns", SortOrder: 12})
	Register(Definition{Group: "campaigns", Slug: "campaigns.view", Name: "View Campaigns", SortOrder: 1})
	Register(Definition{Group: "campaigns", Slug: "campaigns.manage", Name: "Manage Campaigns", SortOrder: 2})
	RegisterGroup(Group{Slug: "flows", Name: "Automation Flows", SortOrder: 13})
	Register(Definition{Group: "flows", Slug: "flows.view", Name: "View Flows", SortOrder: 1})
	Register(Definition{Group: "flows", Slug: "flows.manage", Name: "Manage Flows", SortOrder: 2})
	RegisterGroup(Group{Slug: "emails", Name: "Email Library", SortOrder: 14})
	Register(Definition{Group: "emails", Slug: "emails.view", Name: "View Email Library", SortOrder: 1})
	Register(Definition{Group: "emails", Slug: "emails.manage", Name: "Manage Email Library", SortOrder: 2})
}
