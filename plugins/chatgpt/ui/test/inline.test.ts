import {test} from "node:test";
import assert from "node:assert/strict";
import {mount} from "../src/widget.ts";
import {click,settle,SimExtensions,SimHost,SimPage} from "./sim.ts";
import {cardsOf,cartOf,checkout,imagesOf,items,kettle,labelsOf,marketplace,one,open,orderOf,result,scarf} from "./world.ts";

// The inline view is a small card in the conversation (011:80): at most two actions, no tabs, no deep navigation, no nested
// scrolling, and a carousel of three to eight items with an image and one action each. These tests count what a person sees,
// and they move the widget between the inline view and the fullscreen view.

test("inline, the marketplace is a carousel of at most eight products, each with an image and one action, and it has no tabs, search, or store list",async()=>{
 const {root,host}=await open({products:items(10),stores:marketplace.stores,settings:{mode:"both"}},{},"inline");
 assert.equal(cardsOf(root).length,8,"a carousel holds at most eight items");
 const carousel=root.all().filter(node=>node.className==="grid carousel");
 assert.equal(carousel.length,1,"the inline items sit in one compact carousel");
 assert.equal(carousel[0].attributes.get("role"),"region");
 assert.equal(carousel[0].attributes.get("aria-label"),"Products","a person who cannot see the carousel hears what it is");
 for(const card of cardsOf(root)) {
  assert.equal(imagesOf(card).length,1,"each item has an image");
  assert.ok(imagesOf(card).every(image=>typeof image.alt==="string"&&image.alt.length>0),"and the image has alt text");
  assert.deepEqual(labelsOf(card),["Select"],"and one action");
  assert.deepEqual(card.all().filter(node=>["h3","p"].includes(node.tagName)).map(node=>node.textContent).slice(1),["TEST store","5.00 USD","Available"],"and the merchant, the price, and the availability");
  assert.equal(card.all().filter(node=>node.tagName==="input").length,0,"and no field to fill in");
 }
 assert.ok(root.text().includes("Showing 8 of 10 products."));
 assert.deepEqual(labelsOf(root),[...Array(8).fill("Select"),"Expand"],"the carousel and one control to expand it");
 for(const tag of ["nav","h1","form","table"])assert.equal(root.all().filter(node=>node.tagName===tag).length,0,`an inline view has no ${tag}`);
 for(const label of ["Shop","Cart","Sell","Search","TEST store"])assert.equal(root.buttons(label).length,0,`an inline view has no ${label} button`);
 assert.ok(!root.text().includes("Coming soon"),"the integration overview belongs to the fullscreen view");
 assert.deepEqual(root.all().filter(node=>node.className==="primary").map(node=>node.textContent),Array(8).fill("Select"),"the brand color marks the main action of each item and nothing else");
 assert.equal(host.calls.length,0);

 const few=await open({products:items(2)},{},"inline");
 assert.equal(cardsOf(few.root).length,2);
 assert.ok(!few.root.text().includes("Showing"),"the note appears only when the carousel is cut");
 const none=await open({products:[]},{},"inline");
 assert.ok(none.root.text().includes("No published products match this view."));
});

test("inline, a host that offers only the inline view gets no control to expand",async()=>{
 const host=new SimHost();host.context={availableDisplayModes:["inline"],displayMode:"inline"};
 const page=new SimPage();mount(host,new SimExtensions(),page.asPage());await settle();host.deliver({products:items(3)});await settle();
 assert.deepEqual(labelsOf(page.root),["Select","Select","Select"]);
});

test("inline, a host that does not name its display mode gets the inline view",async()=>{
 const host=new SimHost();host.context=undefined;
 const page=new SimPage();mount(host,new SimExtensions(),page.asPage());await settle();host.deliver({products:items(3)});await settle();
 assert.equal(page.root.all().filter(node=>node.tagName==="nav").length,0);
 assert.deepEqual(labelsOf(page.root),["Select","Select","Select"]);
});

test("Expand asks the host for fullscreen and draws the whole marketplace, and closing it draws the card again with no read of the server",async()=>{
 const {host,root}=await open({products:items(10),stores:marketplace.stores,settings:{mode:"shop"}},{},"inline");
 await click(one(root.buttons("Expand")));
 assert.deepEqual(host.displayModes,["fullscreen"]);
 assert.equal(cardsOf(root).length,10);
 for(const card of cardsOf(root))assert.deepEqual(labelsOf(card),["Select","Ask about this product","Compare"],"the fullscreen view has every product action");
 for(const label of ["Shop","Cart","Sell","Search","TEST store"])one(root.buttons(label));
 const grid=root.all().filter(node=>node.className==="grid");
 assert.equal(grid.length,1,"the fullscreen items sit in the roomy grid");
 assert.equal(grid[0].attributes.has("role"),false);
 assert.equal(root.buttons("Expand").length,0,"the fullscreen view is already expanded");
 assert.ok(root.text().includes("Coming soon: Base, Ethereum, MPP, UCP, ACP, WooCommerce, Claude Code"));

 host.setMode("inline");
 await settle();
 assert.equal(cardsOf(root).length,8);
 assert.deepEqual(labelsOf(root.all().find(node=>node.tagName==="article")!),["Select"]);
 assert.equal(host.calls.length,0,"a change of display mode reads nothing from the server");
});

test("a host that answers the request without announcing the mode still gets the fullscreen view, and a host that refuses keeps the card",async()=>{
 const quiet=await open({products:items(4)},{},"inline");
 quiet.host.announces=false;
 await click(one(quiet.root.buttons("Expand")));
 assert.equal(quiet.root.buttons("Compare").length,4,"the answer of the host decides the view");
 assert.equal(quiet.root.buttons("Expand").length,0);

 const refusing=await open({products:items(4)},{},"inline");
 refusing.host.grants=false;
 await click(one(refusing.root.buttons("Expand")));
 assert.equal(refusing.root.buttons("Compare").length,0);
 assert.deepEqual(labelsOf(refusing.root),["Select","Select","Select","Select","Expand"]);

 // The host's own report is the last word: the quiet host now reports that the person closed the fullscreen view.
 quiet.host.setMode("inline");
 await settle();
 assert.equal(quiet.root.buttons("Compare").length,0);
});

test("a comparison chosen in fullscreen shows no compare action in the inline card",async()=>{
 const products=items(4);
 const {host,root}=await open({products,settings:{mode:"shop"}},{compare_products:()=>result({products})},"fullscreen");
 await click(root.all().find(node=>node.tagName==="article")!.buttons("Compare")[0]);
 assert.equal(root.buttons("Compare 1 products").length,1);
 host.setMode("inline");
 await settle();
 assert.equal(root.buttons("Compare 1 products").length,0);
 assert.ok(labelsOf(root).every(label=>label==="Select"||label==="Expand"));
});

test("inline, Select sets the cart to one and the card becomes the cart",async()=>{
 const {host,root}=await open({products:items(3)},{set_cart:args=>result({cart:cartOf(Number(args.quantity)),product:kettle,checkout_enabled:true})},"inline");
 await click(one(cardsOf(root)[0].buttons("Select")));
 assert.deepEqual(host.calls,[{name:"set_cart",args:{listing_id:"lst_1",variant_id:"lst_1",quantity:1}}]);
 assert.deepEqual(labelsOf(root),["Pay with USDC","Refresh order status"]);
 assert.ok(root.text().includes("TEST ceramic kettle · Quantity 1"));
 const unavailable=await open({products:[{...scarf,images:["https://cdn.shopify.com/a/scarf.png"]}]},{},"inline");
 assert.equal(one(unavailable.root.buttons("Select")).disabled,true);
});

test("inline, the cart shows at most two actions, the ones that the state of the order calls for",async()=>{
 const foreign={...orderOf("paid"),receipt_url:"https://evil.example/orders/ord_7"};
 const states:Array<[string,Record<string,unknown>,string[]]>=[
  ["no cart and no order",{cart:null,checkout_enabled:false},[]],
  ["a cart",{cart:cartOf(2),product:kettle,checkout_enabled:true},["Pay with USDC","Refresh order status"]],
  ["a pending order",{cart:cartOf(2),product:kettle,order:orderOf("pending"),checkout_enabled:false},["Pay with USDC","Refresh order status"]],
  ["a paid order",{cart:cartOf(2),product:kettle,order:orderOf("paid"),checkout_enabled:true},["View receipt"]],
  ["a paid order whose receipt link is not the checkout's",{cart:cartOf(2),product:kettle,order:foreign,checkout_enabled:true},[]],
  ["a pending order and no cart",{cart:null,order:orderOf("pending"),checkout_enabled:false},["Refresh order status"]],
  ["a paid order and no cart",{cart:null,order:orderOf("paid"),checkout_enabled:false},["View receipt"]],
 ];
 for(const [name,state,want] of states) {
  const {root}=await open(state,{},"inline");
  assert.deepEqual(labelsOf(root),want,name);
  assert.ok(labelsOf(root).length<=2,name);
  assert.equal(root.all().filter(node=>node.tagName==="nav").length,0,`${name} has no tabs`);
  assert.ok(root.all().filter(node=>node.className==="primary").length<=1,`${name} has one main action`);
 }
 const pending=await open(states[2][1],{},"inline");
 assert.equal(one(pending.root.buttons("Pay with USDC")).disabled,true,"a pending payment asks for no second payment");
 const full=await open(states[3][1],{},"fullscreen");
 assert.deepEqual(labelsOf(full.root).filter(label=>["View receipt","Pay with USDC","Refresh order status"].includes(label)),["View receipt","Pay with USDC","Refresh order status"],"fullscreen keeps all three");
});

test("inline, the cart pays and refreshes the way the fullscreen cart does",async()=>{
 let state:Record<string,unknown>={cart:cartOf(2),product:kettle,checkout_enabled:true};
 const {host,root}=await open(state,{start_checkout:()=>result({url:checkout}),open_cart:()=>result(state)},"inline");
 await click(one(root.buttons("Pay with USDC")));
 assert.deepEqual(host.names(),["start_checkout"]);
 assert.deepEqual(host.opened,[checkout]);
 state={cart:cartOf(2),product:kettle,order:orderOf("paid"),checkout_enabled:true};
 await click(one(root.buttons("Refresh order status")));
 assert.deepEqual(host.names(),["start_checkout","open_cart"]);
 assert.deepEqual(labelsOf(root),["View receipt"],"after the payment the card offers the receipt alone");
 await click(one(root.buttons("View receipt")));
 assert.deepEqual(host.opened,[checkout,"https://checkout.vadora.xyz/orders/ord_7"]);
});

test("inline, the merchant card has one action, which opens the hosted workspace",async()=>{
 const {host,root}=await open({products:[],settings:{mode:"sell"}},{},"inline");
 assert.deepEqual(labelsOf(root),["Open merchant workspace"]);
 assert.ok(root.text().includes("Connect your invited Shopify store and confirm publication in the hosted workspace."));
 await click(one(root.buttons("Open merchant workspace")));
 assert.deepEqual(host.opened,["https://app.vadora.xyz/"]);
});

test("no view carries the brand name or a logo, and every image has alt text",async()=>{
 const states:unknown[]=[{products:items(3),stores:marketplace.stores,settings:{mode:"both"}},{cart:cartOf(2),product:kettle,order:orderOf("paid"),checkout_enabled:true},{products:[],settings:{mode:"sell"}}];
 for(const mode of ["inline","fullscreen"] as const) {
  for(const state of states) {
   const {root}=await open(state,{},mode);
   assert.ok(!root.text().includes("Vadora"),`a ${mode} view shows no brand name`);
   for(const image of imagesOf(root)) {
    assert.ok(typeof image.alt==="string"&&image.alt.length>0,"each image has alt text");
    assert.ok(String(image.src).startsWith("https://cdn.shopify.com/"),"and is a product image, never a logo");
   }
   for(const heading of root.all().filter(node=>["h1","h2","h3"].includes(node.tagName))) {
    assert.ok(!heading.text().includes("Vadora"),"a heading shows no brand name");
   }
  }
 }
});

test("the widget hands the host context to the page at connection and at every change, and a page that cannot take it changes nothing",async()=>{
 const host=new SimHost();host.context={availableDisplayModes:["inline"],displayMode:"inline",theme:"dark"};
 const page=new SimPage();mount(host,new SimExtensions(),page.asPage());await settle();
 assert.equal(page.styled.length,1);
 assert.equal(page.styled[0]?.theme,"dark");
 host.context={...host.context,theme:"light"};
 host.onhostcontextchanged?.({theme:"light"});
 await settle();
 assert.equal(page.styled.at(-1)?.theme,"light");

 const broken=new SimHost();
 const brokenPage=new SimPage();const asPage=brokenPage.asPage();asPage.styleHost=()=>{throw new Error("the page cannot take the theme");};
 mount(broken,new SimExtensions(),asPage);await settle();
 assert.ok(!brokenPage.root.text().includes("Open Vadora in a supported host"),"a page that cannot take the theme is still connected");
 broken.deliver({products:items(3)});await settle();
 assert.equal(cardsOf(brokenPage.root).length,3);
 broken.onhostcontextchanged?.({theme:"light"});await settle();
 assert.equal(cardsOf(brokenPage.root).length,3,"and it still draws after a change");
});
