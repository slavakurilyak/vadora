import {test} from "node:test";
import assert from "node:assert/strict";
import {mount} from "../src/widget.ts";
import {click,settle,SimExtensions,SimHost,SimPage,type Handler} from "./sim.ts";
import {cartOf,checkout,kettle,marketplace,one,open,orderOf,result,scarf} from "./world.ts";

// These tests run the widget against a simulated host. The widget is the real one: the same code that the conversation
// loads, given a host that answers its tool calls, opens its links, and reports what the model does. They run the fullscreen
// view, which has every control, and inline.test.ts runs the inline view.

test("the global entrypoint renders the marketplace from the initial tool result with no second tool call",async()=>{
 const {host,root}=await open(marketplace);
 assert.equal(host.calls.length,0);
 const page=root.text();
 for(const want of ["Marketplace","TEST ceramic kettle","5.00 USD","Available","TEST wool scarf","25.00 USD","Unavailable","Coming soon: Base, Ethereum, MPP, UCP, ACP, WooCommerce, Claude Code"]) {
  assert.ok(page.includes(want),want);
 }
 one(root.buttons("TEST store"));
 assert.equal(root.buttons("Select").length,2);
 assert.equal(root.buttons("Select").filter(button=>button.disabled).length,1,"an unavailable product cannot be selected");
 assert.equal(root.buttons("Pay with USDC").length,0,"the marketplace is not the cart");
 assert.equal(root.buttons("Refresh order status").length,0,"the marketplace is not the cart");
});

test("the thread entrypoint shows the account's cart and its latest order status",async()=>{
 const {host,root}=await open({cart:cartOf(2),product:kettle,order:orderOf("pending"),checkout_enabled:false,message:"Payment is pending. Refresh the existing status and wait. Pay no second time."});
 assert.equal(host.calls.length,0);
 const page=root.text();
 for(const want of ["Cart","TEST ceramic kettle · Quantity 2","Order ORD-7 · 11.00 USDC · pending · awaiting_payment","Payment is pending. Refresh the existing status and wait. Pay no second time."])
  assert.ok(page.includes(want),want);
 const empty=await open({cart:null,checkout_enabled:false,message:"No new order appears. The order may belong to another email."});
 assert.ok(empty.root.text().includes("Select one product to get started."));
 assert.ok(empty.root.text().includes("The order may belong to another email."));
});

test("the account's mode decides the view, and the toolbar switches between shop, cart, and the merchant workspace",async()=>{
 const sell=await open({products:[],settings:{mode:"sell"}});
 assert.ok(sell.root.text().includes("Merchant workspace"));
 assert.ok(sell.root.text().includes("Connect your invited Shopify store and confirm publication in the hosted workspace."));
 assert.equal(sell.root.buttons("Search").length,0,"the merchant workspace shows no product search");
 await click(one(sell.root.buttons("Sell")));
 assert.deepEqual(sell.host.opened,["https://app.vadora.xyz/"]);
 assert.equal(sell.root.buttons("Expand").length,0,"the fullscreen view is already expanded");

 const both=await open({...marketplace,settings:{mode:"both"}},{
  open_cart:()=>result({cart:cartOf(1),product:kettle,checkout_enabled:true}),
  open_marketplace:()=>result(marketplace),
 });
 for(const label of ["Shop","Cart","Sell"])one(both.root.buttons(label));
 await click(one(both.root.buttons("Cart")));
 assert.deepEqual(both.host.names(),["open_cart"]);
 assert.ok(both.root.text().includes("TEST ceramic kettle · Quantity 1"));
 await click(one(both.root.buttons("Shop")));
 assert.deepEqual(both.host.names(),["open_cart","open_marketplace"]);
 assert.ok(both.root.text().includes("TEST wool scarf"));
});

test("Pay with USDC calls start_checkout and opens only the checkout link that the server returns",async()=>{
 const {host,root}=await open({cart:cartOf(2),product:kettle,checkout_enabled:true},{start_checkout:()=>result({url:checkout})});
 await click(one(root.buttons("Pay with USDC")));
 assert.deepEqual(host.names(),["start_checkout"]);
 assert.deepEqual(host.calls[0].args,{});
 assert.deepEqual(host.opened,[checkout]);

 for(const link of ["https://checkout.vadora.xyz.evil.test/pay?listing=lst_kettle&variant=lst_kettle&quantity=2","http://checkout.vadora.xyz/pay?listing=lst_kettle&variant=lst_kettle&quantity=2",
  "https://checkout.vadora.xyz/pay?listing=lst_kettle&variant=lst_kettle&quantity=2&token=x","https://checkout.vadora.xyz/other","javascript:alert(1)",""]) {
  const hostile=await open({cart:cartOf(2),product:kettle,checkout_enabled:true},{start_checkout:()=>result({url:link})});
  await click(one(hostile.root.buttons("Pay with USDC")));
  assert.deepEqual(hostile.host.opened,[],`opened ${link}`);
  assert.ok(hostile.root.text().includes("Request failed. Refresh the view and try again."));
 }
 const refused=await open({cart:cartOf(2),product:kettle,checkout_enabled:true},{start_checkout:()=>({isError:true,structuredContent:{reason:"no"}})});
 await click(one(refused.root.buttons("Pay with USDC")));
 assert.deepEqual(refused.host.opened,[]);

 const disabled=await open({cart:cartOf(2),product:kettle,checkout_enabled:false},{start_checkout:()=>result({url:checkout})});
 assert.equal(one(disabled.root.buttons("Pay with USDC")).disabled,true);
 await click(one(disabled.root.buttons("Pay with USDC")));
 assert.deepEqual(disabled.host.names(),[],"a disabled button asks the server for nothing");
});

test("a refresh while the payment is pending shows pending and asks for no second payment, and after the payment it shows paid with the receipt",async()=>{
 let state:Record<string,unknown>={cart:cartOf(2),product:kettle,order:orderOf("pending"),checkout_enabled:false,message:"Payment is pending. Refresh the existing status and wait. Pay no second time."};
 const {host,root}=await open(state,{open_cart:()=>result(state),start_checkout:()=>result({url:checkout}),set_cart:()=>result({})});
 await click(one(root.buttons("Refresh order status")));
 assert.deepEqual(host.names(),["open_cart"],"a refresh only reads");
 assert.ok(root.text().includes("pending"));
 assert.equal(one(root.buttons("Pay with USDC")).disabled,true,"a pending payment asks for no second payment");
 assert.equal(root.buttons("View receipt").length,1);

 state={cart:cartOf(2),product:kettle,order:orderOf("paid"),checkout_enabled:true};
 await click(one(root.buttons("Refresh order status")));
 assert.deepEqual(host.names(),["open_cart","open_cart"]);
 assert.ok(root.text().includes("Order ORD-7 · 11.00 USDC · paid · received"));
 await click(one(root.buttons("View receipt")));
 assert.deepEqual(host.opened,["https://checkout.vadora.xyz/orders/ord_7"]);

 for(const receipt of ["https://evil.example/orders/ord_7","https://checkout.vadora.xyz/orders/ord_7?token=x","https://checkout.vadora.xyz/pay","http://checkout.vadora.xyz/orders/ord_7"]) {
  state={cart:cartOf(2),product:kettle,order:{...orderOf("paid"),receipt_url:receipt},checkout_enabled:true};
  await click(one(root.buttons("Refresh order status")));
  assert.equal(root.buttons("View receipt").length,0,`a receipt link ${receipt} is not shown`);
 }
});

test("the open cart rereads the server when a tool result arrives or the host reports a change, and never takes state from a payload",async()=>{
 const server={cart:cartOf(1),product:kettle,order:orderOf("pending"),checkout_enabled:false};
 const {host,extensions,root}=await open({...server},{open_cart:()=>result({...server})});
 assert.ok(root.text().includes("Quantity 1"));

 // The model calls set_cart while the cart is open. The server's cart changes, and the tool result that the host passes on lies.
 server.cart=cartOf(3);
 host.deliver({cart:cartOf(99),order:orderOf("paid"),checkout_enabled:true});
 await settle();
 assert.deepEqual(host.names(),["open_cart"],"a tool result in the thread makes the widget read the server");
 const page=root.text();
 assert.ok(page.includes("Quantity 3"),"the cart shows the new selection that the server returns");
 assert.ok(!page.includes("Quantity 99"),"nothing is taken from the payload");
 assert.ok(page.includes("pending"),"the cart shows pending while the server reports pending");
 assert.ok(!page.includes("paid ·"),"a result that claims paid shows nothing paid");
 assert.equal(one(root.buttons("Pay with USDC")).disabled,true);

 // The host reports a context change that the model made, and the server now says paid.
 server.order=orderOf("paid");server.checkout_enabled=true;
 extensions.current={updateId:"update-made-by-the-model"};
 host.onhostcontextchanged?.();
 await settle();
 assert.deepEqual(host.names(),["open_cart","open_cart"]);
 assert.ok(root.text().includes("paid · received"),"the cart shows paid once the server confirms it");

 // A change that the widget itself published is no reason to read again.
 const before=host.calls.length;
 host.onhostcontextchanged?.();
 await settle();
 assert.equal(host.calls.length,before,"the widget's own context update does not loop");

 for(const call of host.calls)assert.ok(["open_cart","open_marketplace"].includes(call.name),`a reread calls only a read tool, got ${call.name}`);
});

test("a window focus after the first result rereads the server, and before it reads nothing",async()=>{
 const host=new SimHost();host.server={open_marketplace:()=>result(marketplace)};
 const page=new SimPage();mount(host,new SimExtensions(),page.asPage());await settle();
 page.fire("focus");await settle();
 assert.equal(host.calls.length,0,"before the first result there is nothing to read again");
 host.deliver(marketplace);await settle();
 page.fire("focus");await settle();
 assert.deepEqual(host.names(),["open_marketplace"]);
});

test("selecting a product sets the cart on the server, opens the cart from what the server returns, and tells the model only what was selected",async()=>{
 const {host,extensions,root}=await open(marketplace,{set_cart:args=>result({cart:cartOf(Number(args.quantity)),product:kettle,checkout_enabled:true})});
 const card=root.all().find(node=>node.tagName==="article"&&node.text().includes("TEST ceramic kettle"))!;
 const quantity=card.all().find(node=>node.tagName==="input")!;
 quantity.value="2";
 await click(one(card.buttons("Select")));
 assert.deepEqual(host.calls,[{name:"set_cart",args:{listing_id:"lst_kettle",variant_id:"lst_kettle",quantity:2}}]);
 assert.ok(root.text().includes("TEST ceramic kettle · Quantity 2"));
 assert.equal(extensions.updates.length,1);
 assert.deepEqual(extensions.updates[0].structuredContent,{comparison:[],cart:cartOf(2)});
 const blocks=JSON.stringify(extensions.updates[0].content);
 assert.ok(blocks.includes("Selected product: TEST ceramic kettle"));
 for(const word of ["token","delivery","address","bearer"])assert.ok(!blocks.toLowerCase().includes(word),word);

 for(const bad of ["0","101","1.5","","abc"]) {
  const invalid=await open(marketplace,{set_cart:()=>result({cart:cartOf(1),product:kettle})});
  const invalidCard=invalid.root.all().find(node=>node.tagName==="article"&&node.text().includes("TEST ceramic kettle"))!;
  invalidCard.all().find(node=>node.tagName==="input")!.value=bad;
  await click(one(invalidCard.buttons("Select")));
  assert.equal(invalid.host.calls.length,0,`a quantity of ${JSON.stringify(bad)} reaches no server`);
  assert.ok(invalid.root.text().includes("Request failed. Refresh the view and try again."));
 }
});

test("comparing products reads the server's comparison, at most four at a time",async()=>{
 const products=[1,2,3,4,5].map(index=>({...kettle,id:`lst_${index}`,variant_id:`lst_${index}`,title:`TEST item ${index}`}));
 const {host,root}=await open({products,settings:{mode:"shop"}},{compare_products:args=>result({products:products.filter(product=>(args.product_ids as string[]).includes(product.id))})});
 for(const index of [1,2,3,4,5]){
  const card=root.all().find(node=>node.tagName==="article"&&node.text().includes(`TEST item ${index}`))!;
  await click(card.buttons("Compare")[0]??card.buttons("Selected for comparison")[0]);
 }
 await click(one(root.buttons("Compare 4 products")));
 assert.deepEqual(host.calls,[{name:"compare_products",args:{product_ids:["lst_1","lst_2","lst_3","lst_4"]}}]);
 const page=root.text();
 assert.ok(page.includes("Product comparison"));
 assert.ok(page.includes("TEST item 4"));
});

test("the comparison has a row for every attribute that any product has, and says unknown where a product has none",async()=>{
 const red={...kettle,id:"lst_red",variant_id:"lst_red",title:"TEST red kettle",attributes:{Color:"red",Material:"steel"}};
 const plain={...kettle,id:"lst_plain",variant_id:"lst_plain",title:"TEST plain kettle",attributes:{Material:"clay",Weight:"1kg"}};
 const {root}=await open({products:[red,plain],settings:{mode:"shop"}},{compare_products:()=>result({products:[red,plain]})});
 for(const card of root.all().filter(node=>node.tagName==="article")) {
  await click(card.buttons("Compare")[0]);
 }
 await click(one(root.buttons("Compare 2 products")));
 const rows=root.all().filter(node=>node.tagName==="tr").map(row=>row.children.map(cell=>cell.text()));
 assert.deepEqual(rows,[["Title","TEST red kettle","TEST plain kettle"],["Price","5.00 USD","5.00 USD"],["Color","red","unknown"],["Material","steel","clay"],["Weight","unknown","1kg"]]);
 assert.equal(root.all().filter(node=>node.tagName==="caption").map(node=>node.text()).join(),"Product comparison");
});

test("Ask about this product puts the product and its merchant in the composer and nothing else",async()=>{
 const {extensions,root}=await open(marketplace);
 const card=root.all().find(node=>node.tagName==="article"&&node.text().includes("TEST ceramic kettle"))!;
 await click(one(card.buttons("Ask about this product")));
 assert.deepEqual(extensions.sent,[{role:"user",content:[{type:"text",text:"Tell me more about TEST ceramic kettle from TEST store."}]}]);
});

test("only an allowed image of a product is shown, and a product without one shows no image at all",async()=>{
 const {root}=await open({products:[{...kettle,images:["https://evil.example/a.png","https://cdn.shopify.com/a/kettle.png","https://cdn.shopify.com/a/diagram.svg"]}]});
 const images=root.all().filter(node=>node.tagName==="img");
 assert.equal(images.length,1);
 assert.equal(images[0].src,"https://cdn.shopify.com/a/kettle.png");
 assert.equal(images[0].alt,"TEST ceramic kettle");

 const bare=await open({products:[{...scarf,images:["https://evil.example/a.png","http://cdn.shopify.com/a/scarf.png"]},{...kettle,id:"lst_two",variant_id:"lst_two"}]});
 assert.equal(bare.root.all().filter(node=>node.tagName==="img").length,0,"no substitute image stands in for a missing one");
});

test("a product card shows the title, the merchant, the price, and the availability and nothing that the merchant did not state",async()=>{
 const {root}=await open({products:[kettle,scarf]});
 const cards=root.all().filter(node=>node.tagName==="article");
 assert.equal(cards.length,2);
 assert.deepEqual(cards[0].all().filter(node=>["h3","p"].includes(node.tagName)).map(node=>node.textContent),["TEST ceramic kettle","TEST store","5.00 USD","Available"]);
 assert.deepEqual(cards[1].all().filter(node=>["h3","p"].includes(node.tagName)).map(node=>node.textContent),["TEST wool scarf","TEST store","25.00 USD","Unavailable"]);
});

test("a deep link opens the cart, a product, a store, a comparison, or the workspace, and anything else is ignored",async()=>{
 const run=async(url:string,server:Record<string,Handler>)=>{
  const host=new SimHost();host.server=server;
  const extensions=new SimExtensions();extensions.link={url};
  const page=new SimPage();mount(host,extensions,page.asPage());await settle();
  return {host,page};
 };
 const cart=await run("/cart",{open_cart:()=>result({cart:cartOf(1),product:kettle,checkout_enabled:true})});
 assert.deepEqual(cart.host.names(),["open_cart"]);
 assert.ok(cart.page.root.text().includes("Quantity 1"));
 const product=await run("/products/lst_kettle",{get_product:()=>result(kettle)});
 assert.deepEqual(product.host.calls,[{name:"get_product",args:{product_id:"lst_kettle"}}]);
 assert.ok(product.page.root.text().includes("TEST ceramic kettle"));
 const store=await run("/stores/s1",{search_products:()=>result({products:[kettle]})});
 assert.deepEqual(store.host.calls,[{name:"search_products",args:{store_id:"s1"}}]);
 const compare=await run("/compare?ids=lst_kettle,lst_scarf",{compare_products:()=>result({products:[kettle,scarf]})});
 assert.deepEqual(compare.host.calls,[{name:"compare_products",args:{product_ids:["lst_kettle","lst_scarf"]}}]);
 const workspace=await run("/sell",{});
 assert.deepEqual(workspace.host.opened,["https://app.vadora.xyz/"]);
 for(const hostile of ["//evil.test/x","/cart#x","/products/a?token=x","/../secret"]) {
  const ignored=await run(hostile,{});
  assert.deepEqual(ignored.host.calls,[],hostile);
  assert.deepEqual(ignored.host.opened,[],hostile);
 }
});

test("a server error that names the workspace opens the workspace and nothing else",async()=>{
 const {host,root}=await open(marketplace,{search_products:()=>({isError:true,structuredContent:{reason:"confirm in the workspace",workspace_url:"https://app.vadora.xyz/account/agreements"}})});
 await click(one(root.buttons("Search")));
 assert.deepEqual(host.opened,["https://app.vadora.xyz/account/agreements"]);
 const hostile=await open(marketplace,{search_products:()=>({isError:true,structuredContent:{workspace_url:"https://evil.example/account"}})});
 await click(one(hostile.root.buttons("Search")));
 assert.deepEqual(hostile.host.opened,[]);
 assert.ok(hostile.root.text().includes("Request failed. Refresh the view and try again."));
});

test("a host that opens links itself gets the link through its own call and the widget never opens a second one",async()=>{
 const host=new SimHost();host.server={start_checkout:()=>result({url:checkout})};
 const legacy:string[]=[];
 const extensions=new SimExtensions();
 const page=new SimPage();page.window.openai={openExternal:args=>{legacy.push(args.href);}};
 mount(host,extensions,page.asPage());await settle();
 host.deliver({cart:cartOf(2),product:kettle,checkout_enabled:true});await settle();
 await click(one(page.root.buttons("Pay with USDC")));
 assert.deepEqual(legacy,[checkout]);
 assert.deepEqual(host.opened,[]);
});

test("a widget that cannot reach a host says where to go instead",async()=>{
 const host=new SimHost();host.connect=async()=>{throw new Error("no host");};
 const page=new SimPage();mount(host,new SimExtensions(),page.asPage());await settle();
 assert.equal(page.root.text(),"Open Vadora in a supported host, or visit vadora.xyz.");
});
