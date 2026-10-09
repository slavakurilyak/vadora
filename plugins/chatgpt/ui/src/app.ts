import {App} from "@modelcontextprotocol/ext-apps";
import {OpenAIExtensions} from "@openai/mcp-extensions/app";
import {allowedImage,checkoutLink,context,deepLink,externalLink,type Product,type View} from "./state";
const app=new App({name:"Vadora",version:"0.1.0"},{availableDisplayModes:["inline","fullscreen"]});
const extensions=new OpenAIExtensions(app);
const root=document.querySelector<HTMLElement>("#app")!;
let view:View={};
let comparison:string[]=[];
let initial=false;
let openedCart=false;
let updateId:string|undefined;
let changing=false;
let generation=0;
function text(tag:string,value:string,className=""):HTMLElement {
 const node=document.createElement(tag);node.textContent=value;node.className=className;return node;
}
function action(label:string,run:()=>Promise<void>,disabled=false):HTMLButtonElement {
 const button=document.createElement("button");button.textContent=label;button.disabled=disabled;
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
 const legacy=(window as unknown as {openai?:{openExternal?:(args:{href:string})=>void}}).openai;
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
 if(!extensions.modelContext) return;
 const result=await extensions.modelContext.update({structuredContent:context(view,comparison)});
 updateId=result?.updateId;
}
function productCard(product:Product):HTMLElement {
 const card=text("article","","card");
 const image=product.images?.find(allowedImage);
 if(image) {const img=document.createElement("img");img.src=image;img.alt=product.title;img.loading="lazy";card.append(img);}
 card.append(text("h3",product.title),text("p",product.merchant),text("p",`${product.price} ${product.currency}`),text("p",product.available?"Available":"Unavailable"));
 const quantity=document.createElement("input");quantity.type="number";quantity.min="1";quantity.max="100";quantity.value="1";quantity.setAttribute("aria-label","Quantity");card.append(quantity);
 card.append(action("Select",async()=>{const n=Number(quantity.value);if(!Number.isInteger(n)||n<1||n>100)throw new Error("Invalid quantity");view=await call("set_cart",{listing_id:product.id,variant_id:product.variant_id,quantity:n});openedCart=true;render();await publishContext();},!product.available));
 card.append(action(comparison.includes(product.id)?"Selected for comparison":"Compare",async()=>{if(comparison.includes(product.id))comparison=comparison.filter(id=>id!==product.id);else if(comparison.length<4)comparison.push(product.id);render();await publishContext();}));
 return card;
}
function renderProducts(products:Product[]):void {
 const grid=text("div","","grid");for(const product of products.slice(0,50))grid.append(productCard(product));root.append(grid);
 if(products.length===0)root.append(text("p","No published products match this view."));
 if(comparison.length>0)root.append(action(`Compare ${comparison.length} products`,async()=>{view=await call("compare_products",{product_ids:comparison});render();renderComparison(view.products??[]);await publishContext();}));
}
function renderComparison(products:Product[]):void {
 const table=document.createElement("table");table.append(text("caption","Product comparison"));
 const keys=[...new Set(products.flatMap(p=>Object.keys(p.attributes??{})))];
 for(const key of ["Title","Price",...keys]) {
  const row=document.createElement("tr");row.append(text("th",key));
  for(const product of products)row.append(text("td",key==="Title"?product.title:key==="Price"?`${product.price} ${product.currency}`:product.attributes?.[key]??"unknown"));table.append(row);
 }
 root.append(table);
}
function renderCart():void {
 root.append(text("h2","Cart"));if(view.message)root.append(text("p",view.message));
 if(view.product)root.append(text("p",`${view.product.title} · Quantity ${view.cart?.quantity??1}`));
 if(!view.cart)root.append(text("p","Select one product to get started."));
 if(view.order)root.append(text("p",`Order ${view.order.number} · ${view.order.total} · ${view.order.payment_status} · ${view.order.fulfillment_status}`));
 root.append(action("Pay with USDC",async()=>{
  const result=await app.callServerTool({name:"start_checkout",arguments:{}});
  if(result.isError)throw new Error("Checkout unavailable");
  const link=checkoutLink(String((result.structuredContent as {url?:string})?.url??""));
  if(!link)throw new Error("Checkout unavailable");await open(link,"https://checkout.vadora.xyz");
 },view.checkout_enabled!==true));
 root.append(action("Refresh order status",refresh));
}
function render():void {
 root.replaceChildren(text("h1","Vadora"));
 const toolbar=text("nav","");toolbar.append(action("Shop",async()=>{openedCart=false;await refresh();}),action("Cart",async()=>{openedCart=true;await refresh();}),action("Sell",async()=>{await open("https://app.vadora.xyz","https://app.vadora.xyz");}));root.append(toolbar);
 if(app.getHostContext()?.availableDisplayModes?.includes("fullscreen"))toolbar.append(action("Expand",async()=>{await app.requestDisplayMode({mode:"fullscreen"});}));
 if(openedCart) {renderCart();return;}
 if(view.settings?.mode==="sell") {root.append(text("h2","Merchant workspace"),text("p","Connect your invited Shopify store and confirm publication in the hosted workspace."));return;}
 const form=document.createElement("form");const query=document.createElement("input");query.placeholder="Search physical products";query.maxLength=128;query.setAttribute("aria-label","Search products");form.append(query,action("Search",async()=>{view=await call("search_products",{query:query.value,limit:20});render();}));form.addEventListener("submit",e=>e.preventDefault());root.append(form);
 renderProducts(view.products??(view.product?[view.product]:[]));
 for(const store of view.stores??[])root.append(action(store.name,async()=>{view=await call("search_products",{store_id:store.id,limit:20});render();}));
 root.append(text("p","Shopify · x402 · Solana USDC · ChatGPT"),text("p","Coming soon: Base, Ethereum, MPP, UCP, ACP, WooCommerce, Claude Code"));
}
async function navigate(raw:string):Promise<void> {
 const route=deepLink(raw);
 if(route.route==="/cart") {openedCart=true;await refresh();return;}
 if(route.route.startsWith("/sell")) {await open("https://app.vadora.xyz","https://app.vadora.xyz");return;}
 if(route.route==="/products") {const product=await call("get_product",{product_id:route.ids[0]});view={products:[product as unknown as Product]};render();return;}
 if(route.route==="/stores") {view=await call(route.ids.length?"search_products":"list_stores",route.ids.length?{store_id:route.ids[0]}:{});render();return;}
 if(route.route==="/compare") {comparison=route.ids;view=await call("compare_products",{product_ids:comparison});render();renderComparison(view.products??[]);return;}
}
app.ontoolresult=result=>{
 if(initial){void refresh().catch(showError);return;}
 initial=true;view=(result.structuredContent??{}) as View;openedCart=Object.prototype.hasOwnProperty.call(view,"cart");render();
};
app.onhostcontextchanged=()=>{
 if(changing)return;
 const current=extensions.modelContext?.getCurrent();
 if(current?.updateId===updateId)return;
 changing=true;void refresh().catch(showError).finally(()=>{changing=false;});
};
window.addEventListener("focus",()=>{if(initial)void refresh().catch(showError);});
app.connect().then(async()=>{const link=extensions.deepLink.getCurrent();if(link)await navigate(link.url);}).catch(()=>{root.replaceChildren(text("p","Open Vadora in a supported host, or visit vadora.xyz."));});
