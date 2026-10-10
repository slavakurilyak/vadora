export type Product = {id:string; variant_id:string; title:string; merchant:string; price:string; currency:string; available:boolean; product_url?:string; images?:string[]; attributes?:Record<string,string>};
export type Cart = {listing_id:string; variant_id:string; quantity:number};
export type SafeOrder = {number:string; total:string; payment_status:string; fulfillment_status:string; receipt_url?:string};
export type View = {products?:Product[]; stores?:{id:string;name:string;url:string}[]; cart?:Cart|null; product?:Product; order?:SafeOrder; checkout_enabled?:boolean; message?:string; workspace_url?:string; settings?:{mode:string}; integrations?:{name:string;status:string}[]};
const identifier = /^[a-zA-Z0-9_-]{1,64}$/;
export function deepLink(raw:string): {route:string;ids:string[]} {
 const root = {route:"/",ids:[]};
 if(raw.length>1024 || !raw.startsWith("/") || raw.startsWith("//")) return root;
 try {
  const u=new URL(raw,"https://ui.vadora.xyz");
  if(u.hash || u.origin!=="https://ui.vadora.xyz") return root;
  const path=decodeURIComponent(u.pathname);
  if(path.includes("%")) return root;
  if(path==="/compare") {
   if([...u.searchParams.keys()].length!==1) return root;
   const ids=(u.searchParams.get("ids")??"").split(",");
   if(ids.length>4 || ids.some(id=>!identifier.test(id))) return root;
   return {route:path,ids};
  }
  if(u.search) return root;
  if(["/","/stores","/cart","/sell","/sell/readiness"].includes(path)) return {route:path,ids:[]};
  const match=/^\/(products|stores)\/([a-zA-Z0-9_-]{1,64})$/.exec(path);
  if(match) return {route:"/"+match[1],ids:[match[2]]};
 } catch { return root; }
 return root;
}
export function allowedImage(raw:string): boolean {
 try {
  const u=new URL(raw);
  if(u.protocol!=="https:" || u.username || u.password || u.port) return false;
  if(!["cdn.shopify.com","assets.vadora.xyz"].includes(u.hostname)) return false;
  return /\.(png|jpe?g|webp|gif|avif)$/i.test(u.pathname);
 } catch { return false; }
}
export function externalLink(raw:string,origin:string): string|undefined {
 try {
  const u=new URL(raw);
  if(u.origin!==origin || u.username || u.password || u.hash) return;
  if(u.protocol!=="https:") return;
  return u.href;
 } catch { return; }
}
export function checkoutLink(raw:string): string|undefined {
 const safe=externalLink(raw,"https://checkout.vadora.xyz");
 if(!safe) return;
 const u=new URL(safe);
 if(u.pathname!=="/pay") return;
 if([...u.searchParams.keys()].length!==3) return;
 const listing=u.searchParams.get("listing")??"";
 const variant=u.searchParams.get("variant")??"";
 const quantity=u.searchParams.get("quantity")??"";
 if(!identifier.test(listing) || !identifier.test(variant)) return;
 if(!/^[1-9][0-9]?$|^100$/.test(quantity)) return;
 return safe;
}
export function context(view:View,comparison:string[]): Record<string,unknown> {
 const value:Record<string,unknown>={comparison:comparison.filter(id=>identifier.test(id)).slice(0,4)};
 if(view.cart) value.cart={listing_id:view.cart.listing_id,variant_id:view.cart.variant_id,quantity:view.cart.quantity};
 if(view.order) value.order={number:view.order.number,total:view.order.total,payment_status:view.order.payment_status,fulfillment_status:view.order.fulfillment_status};
 return value;
}

export type Block={type:"text";text:string};
// secretLike matches what must never reach the model or the composer: an address, a token, or a long opaque run.
const secretLike=/\b(?:0x[0-9a-fA-F]{32,}|[1-9A-HJ-NP-Za-km-z]{32,}|[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,})\b/g;
// plain keeps merchant supplied text short, free of control characters, and free of anything that looks like an address or a token.
export function plain(value:string,limit=200):string {
 return String(value).replace(secretLike,"[removed]").replace(/[\u0000-\u001f\u007f]/g," ").slice(0,limit);
}
// contextBlocks are the titled text blocks that tell the model what the shopper selected and what an order shows (011:148).
// They hold listing data and the order number, total, and status, and never a delivery detail, a link, or a credential.
export function contextBlocks(view:View,selected:Product[]):Block[] {
 const blocks:Block[]=[];
 for(const product of selected.slice(0,4)) blocks.push({type:"text",text:`Selected product: ${plain(product.title)}\nMerchant: ${plain(product.merchant)}\nPrice: ${plain(product.price,32)} ${plain(product.currency,8)}\n${product.available?"Available":"Unavailable"}`});
 if(view.order) blocks.push({type:"text",text:`Order ${plain(view.order.number,32)}: total ${plain(view.order.total,32)}, payment ${plain(view.order.payment_status,32)}, fulfillment ${plain(view.order.fulfillment_status,32)}.`});
 return blocks;
}
// composerMessage is the text that "Ask about this product" sends. It names the product and its merchant and nothing else:
// no address, no token, and no quote amount (011:161).
export function composerMessage(product:Product):Block[] {
 return [{type:"text",text:`Tell me more about ${plain(product.title)} from ${plain(product.merchant)}.`}];
}
