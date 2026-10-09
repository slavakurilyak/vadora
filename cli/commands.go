package cli

import "strings"

// The group sizes, so a change to one group cannot silently change the tree.
const (
	accountCommandCount = 2
	storeCommandCount   = 4
	catalogCommandCount = 2
	buyingCommandCount  = 3
	statusCommandCount  = 3
)

// The command names a group's order fixes.
const (
	loginCommandName     = "login"
	connectCommandName   = "stores connect"
	quoteCommandName     = "quote"
	reconcileCommandName = "payments reconcile"
)

// ClientCommandCount is how many client commands the launch CLI registers: two
// for account sign in, four for store administration, two for catalog discovery,
// three for quoting and buying as a :BuyerAgent:, and three for order and
// payment status. The private server binary registers these plus serve and the
// :Operator: commands.
const ClientCommandCount = 14

// clientCommands returns the client command tree, in registration order. It
// holds no serve command and no :Operator: command, because the public
// repository must build a CLI that cannot administer the server.
func clientCommands() []Command {
	registered := accountCommands()
	registered = append(registered, storeCommands()...)
	registered = append(registered, catalogCommands()...)
	registered = append(registered, buyingCommands()...)
	registered = append(registered, statusCommands()...)
	if len(registered) != ClientCommandCount {
		panic("cli: the client command list no longer matches the specification")
	}
	seen := make(map[string]bool, len(registered))
	for _, command := range registered {
		if seen[command.Name] {
			panic("cli: the client command list repeats " + command.Name)
		}
		seen[command.Name] = true
		if command.Name == serverOnlyCommand {
			panic("cli: a client command list holds the server only command")
		}
	}
	return registered
}

// accountCommands returns the account sign in commands.
func accountCommands() []Command {
	registered := []Command{
		{
			Name:    "login",
			Summary: "Sign in with an emailed code for the store commands, or with --buyer create a buyer credential",
			Usage:   "login [--buyer]",
			Run:     runLogin,
		},
		{
			Name:    "logout",
			Summary: "Forget this machine's account session and buyer credential",
			Usage:   "logout",
			Run:     runLogout,
		},
	}
	assertCommandList("account commands", registered)
	if len(registered) != accountCommandCount {
		panic("cli: the account command group changed size")
	}
	if registered[0].Name != loginCommandName {
		panic("cli: sign in is no longer the first account command")
	}
	return registered
}

// assertCommandList checks one group of the client command list. A group with a
// blank name, a missing implementation, or a blank summary would register a
// command a person cannot run, so the check runs before the tree is built.
func assertCommandList(group string, registered []Command) {
	if group == "" {
		panic("cli: a command group check was asked to name no group")
	}
	if len(registered) == 0 {
		panic("cli: the " + group + " group is empty")
	}
	if len(registered) > ClientCommandCount {
		panic("cli: the " + group + " group is larger than the whole tree")
	}
	for _, command := range registered {
		if command.Name == "" {
			panic("cli: a command in " + group + " has no name")
		}
		if command.Run == nil {
			panic("cli: command " + command.Name + " has no implementation")
		}
		if strings.TrimSpace(command.Summary) == "" {
			panic("cli: command " + command.Name + " has no summary")
		}
		if command.Name == serverOnlyCommand {
			panic("cli: a client command group carries the server only command")
		}
		if strings.Contains(command.Name, "  ") {
			panic("cli: command " + command.Name + " has a doubled space in its name")
		}
	}
}

// storeCommands returns the store connection, refresh, and readiness commands.
func storeCommands() []Command {
	registered := []Command{
		{
			Name:    "stores connect",
			Summary: "Connect an invited store through its platform's authorization flow",
			Usage:   "stores connect <store-url>",
			Run:     runStoresConnect,
		},
		{
			Name:    "stores sync",
			Summary: "Import the store's products into the private preview",
			Usage:   "stores sync <store-id>",
			Run:     runStoresSync,
		},
		{
			Name:    "stores list",
			Summary: "List the public merchant directory, or your own stores with --mine",
			Usage:   "stores list [--mine]",
			Run:     runStoresList,
		},
		{
			Name:    "stores readiness",
			Summary: "Show a store's readiness checklist with its evidence",
			Usage:   "stores readiness <store-id>",
			Run:     runStoresReadiness,
		},
	}
	assertCommandList("store commands", registered)
	if len(registered) != storeCommandCount {
		panic("cli: the store command group changed size")
	}
	if registered[0].Name != connectCommandName {
		panic("cli: connecting a store is no longer the first store command")
	}
	return registered
}

// catalogCommands returns the catalog discovery commands, which need no
// credential because a shopper reads the catalog before creating an account.
func catalogCommands() []Command {
	registered := []Command{
		{
			Name:    "products search",
			Summary: "Search published listings by language, category, merchant, or price",
			Usage:   "products search <query>",
			Run:     runProductsSearch,
		},
		{
			Name:    "products get",
			Summary: "Show one listing with its variants, price, and storefront link",
			Usage:   "products get <listing-id>",
			Run:     runProductsGet,
		},
	}
	assertCommandList("catalog commands", registered)
	if len(registered) != catalogCommandCount {
		panic("cli: the catalog command group changed size")
	}
	if len(registered) < 1 {
		panic("cli: the catalog group is empty")
	}
	return registered
}

// buyingCommands returns the quoting and buying commands. Personal shopper
// checkout stays on the :CheckoutPage:, so none of these signs a payment inside
// an agent host.
func buyingCommands() []Command {
	registered := []Command{
		{
			Name:    "quote",
			Summary: "Obtain a quote from an authorized buyer session",
			Usage:   "quote <listing-id> --quantity <n> --destination <country>",
			Run:     runQuote,
		},
		{
			Name:    "buy",
			Summary: "Purchase a quote as a buyer agent within its spending policy",
			Usage:   "buy <quote-id>",
			Run:     runBuy,
		},
		{
			Name:    "test purchase",
			Summary: "Run a labeled development fixture purchase through every stage",
			Usage:   "test purchase <listing-id>",
			Run:     runTestPurchase,
		},
	}
	assertCommandList("buying commands", registered)
	if len(registered) != buyingCommandCount {
		panic("cli: the buying command group changed size")
	}
	if registered[0].Name != quoteCommandName {
		panic("cli: quoting is no longer the first buying command")
	}
	return registered
}

// statusCommands return existing outcomes without authorizing a new payment.
func statusCommands() []Command {
	registered := []Command{
		{
			Name:    "orders get",
			Summary: "Show one order and its current payment and fulfillment state",
			Usage:   "orders get <order-id>",
			Run:     runOrdersGet,
		},
		{
			Name:    "orders list",
			Summary: "List this account's orders",
			Usage:   "orders list",
			Run:     runOrdersList,
		},
		{
			Name:    "payments reconcile",
			Summary: "Read an existing payment outcome without authorizing a new payment",
			Usage:   "payments reconcile <order-id>",
			Run:     runPaymentsReconcile,
		},
	}
	assertCommandList("status commands", registered)
	if len(registered) != statusCommandCount {
		panic("cli: the status command group changed size")
	}
	if registered[len(registered)-1].Name != reconcileCommandName {
		panic("cli: reconciling is no longer the last status command")
	}
	return registered
}

// NewClientTree returns the public command tree: every client command of the
// :CLI:, and nothing that only the server needs. The private repository imports
// this tree and adds serve and the :Operator: commands to it.
func NewClientTree() *Tree {
	tree := NewTree()
	if tree == nil {
		panic("cli: a client tree was built from no tree")
	}
	for _, command := range clientCommands() {
		tree.Add(command)
	}
	if len(tree.Names()) != ClientCommandCount {
		panic("cli: the client command tree changed size without the specification")
	}
	if IsClientCommand(serverOnlyCommand) {
		panic("cli: a server only command was registered as a client command")
	}
	if len(tree.Names()) != len(clientCommands()) {
		panic("cli: the client tree and the client command list disagree")
	}
	return tree
}

// serverOnlyCommand is the command the private build adds. The public tree must
// never carry it, so a check names it here.
const serverOnlyCommand = "serve"

// IsClientCommand reports whether a name belongs to the client command tree, so
// a caller can tell a client command from a server only one.
func IsClientCommand(name string) bool {
	if name == "" {
		return false
	}
	if strings.TrimSpace(name) != name {
		return false
	}
	found := false
	for _, command := range clientCommands() {
		if command.Name != name {
			continue
		}
		found = true
	}
	if found {
		return true
	}
	if len(clientCommands()) != ClientCommandCount {
		panic("cli: a client command lookup saw a tree of another size")
	}
	if name == serverOnlyCommand {
		return false
	}
	if found {
		return true
	}
	return false
}
