import {allowedImage,checkoutLink,composerMessage,context,contextBlocks,deepLink,externalLink,receiptLink,type Product,type View} from "./state.ts";
// These are the parts of the MCP Apps host, of the OpenAI extensions, and of the page that the widget uses. The browser
// entry passes the real ones, and a test passes a simulated host, so the widget runs in a test exactly as it runs in the
// conversation (011:106).
export type ToolResult={isError?:boolean;structuredContent?:unknown};
// HostContext is the part of the MCP Apps host context that the widget reads, plus the theme and the style variables that it
// passes on to the page.
export type HostContext={availableDisplayModes?:string[];displayMode?:string;theme?:string;styles?:unknown};
export type Host={
 callServerTool(params:{name:string;arguments:Record<string,unknown>}):Promise<ToolResult>;
 openLink(params:{url:string}):Promise<unknown>;
 getHostContext():HostContext|undefined;
 requestDisplayMode(params:{mode:"inline"|"fullscreen"}):Promise<{mode?:string}|undefined>;
 connect():Promise<void>;
 ontoolresult:((result:ToolResult)=>void)|undefined;
 onhostcontextchanged:((changed?:HostContext)=>void)|undefined;
};
export type Extensions={
 modelContext?:{update(params:{content:unknown;structuredContent:unknown}):Promise<{updateId?:string}|undefined>;getCurrent():{updateId?:string}|undefined};
 message?:{send(params:{role:"user";content:unknown}):Promise<unknown>};
 deepLink:{getCurrent():{url:string}|undefined};
};
export type Page={
 document:Document;
 window:{openai?:{openExternal?:(args:{href:string})=>void};addEventListener(type:string,listener:()=>void):void};
 // styleHost gives the page the theme and the style variables of the host, so that the widget takes its colors and its
 // font from the conversation around it (011:80). The browser entry passes the MCP Apps helpers.
 styleHost?:(context:HostContext|undefined)=>void;
};
// The widget draws one state in two views (011:80). Inline, in the conversation, it is a small card: a carousel of at most
// eight products with one action each, or a cart with at most two actions, and it has no tabs, no search, and no store
// list. Fullscreen, it is the whole marketplace with the toolbar, the search, the stores, and every product action.
const carouselLimit=8;
const gridLimit=50;
type Mode="inline"|"fullscreen";
// element makes one element of the page with its text and its class.
function element(page:Page,tag:string,value:string,className=""):HTMLElement {
 const node=page.document.createElement(tag);node.textContent=value;node.className=className;return node;
}
// comparisonTable lays the chosen products side by side, with one row for each attribute that any of them has.
function comparisonTable(page:Page,products:Product[]):HTMLElement {
 const table=page.document.createElement("table");table.append(element(page,"caption","Product comparison"));
 const keys=[...new Set(products.flatMap(p=>Object.keys(p.attributes??{})))];
 for(const key of ["Title","Price",...keys]) {
  const row=page.document.createElement("tr");row.append(element(page,"th",key));
  for(const product of products)row.append(element(page,"td",key==="Title"?product.title:key==="Price"?`${product.price} ${product.currency}`:product.attributes?.[key]??"unknown"));table.append(row);
 }
 return table;
}
// canExpand tells whether the host offers the fullscreen view.
function canExpand(app:Host):boolean {return app.getHostContext()?.availableDisplayModes?.includes("fullscreen")===true;}
export function mount(app:Host,extensions:Extensions,page:Page):void {
 const root=page.document.querySelector<HTMLElement>("#app")!;
 let view:View={};
 let comparison:string[]=[];
 let initial=false;
 let openedCart=false;
 let updateId:string|undefined;
 let changing=false;
 let generation=0;
 let drawn:Mode|undefined;
 let granted:Mode|undefined;
 const seen=new Map<string,Product>();
 function remember(products:Product[]):void {for(const product of products)seen.set(product.id,product);}
 function selectedProducts():Product[] {
  const ids=new Set<string>(comparison);
  if(view.cart)ids.add(view.cart.listing_id);
  return [...ids].map(id=>seen.get(id)).filter((product):product is Product=>product!==undefined);
 }
 // mode is the view that the host shows. It is the mode that the host answered to the last request for fullscreen, until the
 // host reports a mode of its own, and then the mode in the host context. A host that says nothing is treated as inline,
 // the smaller view.
 function mode():Mode {
  if(granted!==undefined)return granted;
  return app.getHostContext()?.displayMode==="fullscreen"?"fullscreen":"inline";
 }
 // redraw draws the same state again when the host has changed the display mode. It reads nothing from the server.
 function redraw():void {if(drawn!==undefined&&mode()!==drawn)render();}
 async function expand():Promise<void> {
  const result=await app.requestDisplayMode({mode:"fullscreen"});
  if(result?.mode!==undefined)granted=result.mode==="fullscreen"?"fullscreen":"inline";
  redraw();
 }
 // styleHost passes the host's theme and style variables to the page. A page that cannot take them keeps its own palette.
 function styleHost():void {try{page.styleHost?.(app.getHostContext());}catch{/* the fallback palette stays */}}
 function text(tag:string,value:string,className=""):HTMLElement {return element(page,tag,value,className);}
 // action is a button. The one main action of a card is primary, and the brand color appears on primary buttons only.
 function action(label:string,run:()=>Promise<void>,disabled=false,primary=false):HTMLButtonElement {
  const button=page.document.createElement("button");button.textContent=label;button.disabled=disabled;
  if(primary)button.className="primary";
  button.addEventListener("click",()=>{button.disabled=true;void run().catch(()=>showError()).finally(()=>{button.disabled=disabled;});});return button;
 }
 function showError():void {root.append(text("p","Request failed. Refresh the view and try again.","error"));}
 async function call(name:string,args:Record<string,unknown>={}):Promise<View> {
  const result=await app.callServerTool({name,arguments:args});
  if(result.isError) {
   const data=result.structuredContent as {reason?:string;workspace_url?:string}|undefined;
   if(data?.workspace_url) await open(data.workspace_url,"https://app.vadora.xyz");
   throw new Error("Vadora request failed");
  }
  return (result.structuredContent??{}) as View;
 }
 async function open(raw:string,origin:string):Promise<void> {
  const link=externalLink(raw,origin);if(!link) throw new Error("Invalid link");
  const legacy=page.window.openai;
  if(legacy?.openExternal) {legacy.openExternal({href:link});return;}
  await app.openLink({url:link});
 }
 async function refresh():Promise<void> {
  const current=++generation;
  const next=await call(openedCart?"open_cart":"open_marketplace");
  if(current!==generation) return;
  view=next;render();await publishContext();
 }
 async function publishContext():Promise<void> {
  remember([...(view.products??[]),...(view.product?[view.product]:[])]);
  if(!extensions.modelContext) return;
  const result=await extensions.modelContext.update({content:contextBlocks(view,selectedProducts()),structuredContent:context(view,comparison)});
  updateId=result?.updateId;
 }
 async function select(product:Product,quantity:number):Promise<void> {
  if(!Number.isInteger(quantity)||quantity<1||quantity>100)throw new Error("Invalid quantity");
  view=await call("set_cart",{listing_id:product.id,variant_id:product.variant_id,quantity});openedCart=true;render();await publishContext();
 }
 async function openWorkspace():Promise<void> {await open("https://app.vadora.xyz","https://app.vadora.xyz");}
 // productCard is one carousel item inline, with its image and one action, and a product of the grid in fullscreen, with
 // the quantity, the question for the model, and the comparison.
 function productCard(product:Product,full:boolean):HTMLElement {
  const card=text("article","","card");
  const image=product.images?.find(allowedImage);
  if(image) {const img=page.document.createElement("img");img.src=image;img.alt=product.title;img.loading="lazy";card.append(img);}
  card.append(text("h2",product.title),text("p",product.merchant),text("p",`${product.price} ${product.currency}`),text("p",product.available?"Available":"Unavailable"));
  const actions=text("div","","actions");
  if(!full) {actions.append(action("Select",()=>select(product,1),!product.available,true));card.append(actions);return card;}
  const quantity=page.document.createElement("input");quantity.type="number";quantity.min="1";quantity.max="100";quantity.value="1";quantity.setAttribute("aria-label","Quantity");card.append(quantity);
  actions.append(action("Select",()=>select(product,Number(quantity.value)),!product.available,true));
  if(extensions.message)actions.append(action("Ask about this product",async()=>{await extensions.message?.send({role:"user",content:composerMessage(product)});}));
  actions.append(action(comparison.includes(product.id)?"Selected for comparison":"Compare",async()=>{if(comparison.includes(product.id))comparison=comparison.filter(id=>id!==product.id);else if(comparison.length<4)comparison.push(product.id);render();await publishContext();}));
  card.append(actions);
  return card;
 }
 function renderProducts(products:Product[],full:boolean):void {
  const visible=products.slice(0,full?gridLimit:carouselLimit);
  const grid=text("div","",full?"grid":"grid carousel");
  if(!full) {grid.setAttribute("role","region");grid.setAttribute("aria-label","Products");}
  for(const product of visible)grid.append(productCard(product,full));root.append(grid);
  if(products.length===0)root.append(text("p","No published products match this view."));
  if(products.length>visible.length)root.append(text("p",`Showing ${visible.length} of ${products.length} products.`,"muted"));
  if(!full&&canExpand(app))root.append(action("Expand",expand));
  if(full&&comparison.length>0)root.append(action(`Compare ${comparison.length} products`,async()=>{view=await call("compare_products",{product_ids:comparison});render();renderComparison(view.products??[]);await publishContext();}));
 }
 function renderComparison(products:Product[]):void {root.append(comparisonTable(page,products));}
 // The actions of the cart. Fullscreen shows all three. Inline shows at most two, the ones that the order's state calls
 // for: a paid order shows its receipt alone, and an unpaid cart shows the payment and the refresh.
 function renderCart(full:boolean):void {
  root.append(text("h2","Cart"));if(view.message)root.append(text("p",view.message));
  if(view.product)root.append(text("p",`${view.product.title} · Quantity ${view.cart?.quantity??1}`));
  if(!view.cart)root.append(text("p","Select one product to get started."));
  if(view.order)root.append(text("p",`Order ${view.order.number} · ${view.order.total} · ${view.order.payment_status} · ${view.order.fulfillment_status}`));
  const receipt=view.order?.receipt_url;
  const receiptUrl=receipt?receiptLink(receipt):undefined;
  const viewReceipt=()=>action("View receipt",async()=>{await open(receiptUrl!,"https://checkout.vadora.xyz");});
  const pay=()=>action("Pay with USDC",async()=>{
   const result=await app.callServerTool({name:"start_checkout",arguments:{}});
   if(result.isError)throw new Error("Checkout unavailable");
   const link=checkoutLink(String((result.structuredContent as {url?:string})?.url??""));
   if(!link)throw new Error("Checkout unavailable");await open(link,"https://checkout.vadora.xyz");
  },view.checkout_enabled!==true,true);
  const refreshStatus=()=>action("Refresh order status",refresh);
  const actions:HTMLElement[]=[];
  if(full) {
   if(receiptUrl)actions.push(viewReceipt());
   actions.push(pay(),refreshStatus());
  } else if(view.order?.payment_status==="paid") {
   if(receiptUrl)actions.push(viewReceipt());
  } else {
   if(view.cart)actions.push(pay());
   if(view.cart||view.order)actions.push(refreshStatus());
  }
  if(actions.length>0) {const row=text("div","","actions");row.append(...actions);root.append(row);}
 }
 function renderSearch():void {
  const form=page.document.createElement("form");const query=page.document.createElement("input");query.placeholder="Search physical products";query.maxLength=128;query.setAttribute("aria-label","Search products");
  form.append(query,action("Search",async()=>{view=await call("search_products",{query:query.value,limit:20});render();}));form.addEventListener("submit",e=>e.preventDefault());root.append(form);
 }
 function renderStores():void {
  const stores=view.stores??[];
  if(stores.length===0)return;
  const row=text("div","","actions");
  for(const store of stores)row.append(action(store.name,async()=>{view=await call("search_products",{store_id:store.id,limit:20});render();}));
  root.append(row);
 }
 // renderMerchant is the merchant view. Inline it carries the one action that opens the hosted workspace, because the
 // inline view has no toolbar to reach it.
 function renderMerchant(full:boolean):void {
  root.append(text("h2","Merchant workspace"),text("p","Connect your invited Shopify store and confirm publication in the hosted workspace."));
  if(!full)root.append(action("Open merchant workspace",openWorkspace,false,true));
 }
 function renderFullscreen():void {
  root.append(text("h1","Marketplace"));
  const toolbar=text("nav","");toolbar.append(action("Shop",async()=>{openedCart=false;await refresh();}),action("Cart",async()=>{openedCart=true;await refresh();}),action("Sell",openWorkspace));root.append(toolbar);
  if(openedCart) {renderCart(true);return;}
  if(view.settings?.mode==="sell") {renderMerchant(true);return;}
  renderSearch();
  renderProducts(view.products??(view.product?[view.product]:[]),true);
  renderStores();
  root.append(text("p","Shopify · x402 · Solana USDC · ChatGPT"),text("p","Coming soon: Base, Ethereum, MPP, UCP, ACP, WooCommerce, Claude Code"));
 }
 function renderInline():void {
  if(openedCart) {renderCart(false);return;}
  if(view.settings?.mode==="sell") {renderMerchant(false);return;}
  renderProducts(view.products??(view.product?[view.product]:[]),false);
 }
 function render():void {
  drawn=mode();
  root.replaceChildren();
  if(drawn==="fullscreen") {renderFullscreen();return;}
  renderInline();
 }
 async function navigate(raw:string):Promise<void> {
  const route=deepLink(raw);
  if(route.route==="/cart") {openedCart=true;await refresh();return;}
  if(route.route.startsWith("/sell")) {await openWorkspace();return;}
  if(route.route==="/products") {const product=await call("get_product",{product_id:route.ids[0]});view={products:[product as unknown as Product]};render();return;}
  if(route.route==="/stores") {view=await call(route.ids.length?"search_products":"list_stores",route.ids.length?{store_id:route.ids[0]}:{});render();return;}
  if(route.route==="/compare") {comparison=route.ids;view=await call("compare_products",{product_ids:comparison});render();renderComparison(view.products??[]);return;}
 }
 app.ontoolresult=result=>{
  if(initial){void refresh().catch(showError);return;}
  initial=true;view=(result.structuredContent??{}) as View;openedCart=Object.prototype.hasOwnProperty.call(view,"cart");render();
 };
 app.onhostcontextchanged=changed=>{
  if(changed?.displayMode!==undefined)granted=undefined;
  styleHost();
  redraw();
  if(changing)return;
  const current=extensions.modelContext?.getCurrent();
  if(current?.updateId===updateId)return;
  changing=true;void refresh().catch(showError).finally(()=>{changing=false;});
 };
 page.window.addEventListener("focus",()=>{if(initial)void refresh().catch(showError);});
 app.connect().then(async()=>{styleHost();const link=extensions.deepLink.getCurrent();if(link)await navigate(link.url);}).catch(()=>{root.replaceChildren(text("p","Open Vadora in a supported host, or visit vadora.xyz."));});
}
