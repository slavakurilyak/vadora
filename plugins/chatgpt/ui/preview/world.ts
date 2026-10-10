// This is the fake Vadora server behind the preview host. It answers the tool calls of the widget with a small catalog of
// labeled TEST products, one cart, and one order whose payment status the person at the preview chooses, so every view of
// the widget can be opened and clicked without a real store, a real wallet, or a real ChatGPT.

export type Json=Record<string,unknown>;
export type Scenario="shop"|"both"|"sell"|"cart-empty"|"cart";
export type OrderState="none"|"pending"|"paid";
export type Reply={isError?:boolean;structuredContent?:Json};

type Product={id:string;variant_id:string;title:string;merchant:string;price:string;currency:string;available:boolean;images:string[];attributes:Record<string,string>};

// The images are Shopify's own sample images, which the widget's image allowlist accepts.
const photos=["garnished","bath","shoes","watch"].map(name=>`https://cdn.shopify.com/static/sample-images/${name}.jpeg`);
const titles=["Ceramic kettle","Linen bath set","Trail shoes","Steel field watch","Stoneware mug","Wool scarf","Walnut cutting board","Brass desk lamp","Cotton tote","Enamel pot"];
const materials=["Stoneware","Linen","Leather","Steel","Stoneware","Wool","Walnut","Brass","Cotton","Enamel"];

export const products:Product[]=titles.map((title,index)=>({
 id:`lst_${index+1}`,variant_id:`var_${index+1}`,title:`TEST ${title}`,merchant:index%2===0?"TEST Oak Store":"TEST Harbor Store",
 price:(5+index*4.5).toFixed(2),currency:"USD",available:index!==5,images:[photos[index%photos.length]],
 attributes:{Material:materials[index],Origin:index%3===0?"Portugal":"Japan"},
}));

const stores=[{id:"shop_1",name:"TEST Oak Store",url:"https://vadora.xyz/stores/shop_1"},{id:"shop_2",name:"TEST Harbor Store",url:"https://vadora.xyz/stores/shop_2"}];
const checkout="https://checkout.vadora.xyz/pay";

export class World {
 cart:{listing_id:string;variant_id:string;quantity:number}|null=null;
 order:OrderState="none";
 mode:"shop"|"sell"|"both"="shop";
 constructor(scenario:Scenario,order:OrderState) {
  this.order=order;
  if(scenario==="shop"||scenario==="both"||scenario==="sell")this.mode=scenario;
  if(scenario==="cart")this.cart={listing_id:products[0].id,variant_id:products[0].variant_id,quantity:2};
  if(order!=="none"&&this.cart===null)this.cart={listing_id:products[0].id,variant_id:products[0].variant_id,quantity:2};
 }
 private find(id:unknown):Product|undefined {return products.find(product=>product.id===id);}
 private cartView():Json {
  const view:Json={cart:this.cart,checkout_enabled:this.cart!==null&&this.order!=="pending"};
  const product=this.cart?this.find(this.cart.listing_id):undefined;
  if(product)view.product=product;
  if(this.order!=="none"&&product&&this.cart) {
   const total=(Number(product.price)*this.cart.quantity).toFixed(2);
   view.order={number:"ORD-7",total:`${total} USDC`,payment_status:this.order,fulfillment_status:this.order==="paid"?"received":"awaiting_payment",receipt_url:"https://checkout.vadora.xyz/orders/ord_7"};
  }
  if(this.order==="pending")view.message="Payment is pending. Refresh the existing status and wait. Pay no second time.";
  return view;
 }
 // initial is the result of the tool call that opens the widget: the cart for a cart scenario, and the marketplace otherwise.
 initial(scenario:Scenario):Json {
  if(scenario==="cart"||scenario==="cart-empty")return this.cartView();
  return this.marketplace();
 }
 private marketplace():Json {return {products,stores,settings:{mode:this.mode}};}
 call(name:string,args:Json):Reply {
  if(name==="open_marketplace")return {structuredContent:this.marketplace()};
  if(name==="open_cart")return {structuredContent:this.cartView()};
  if(name==="list_stores")return {structuredContent:{stores}};
  if(name==="search_products")return {structuredContent:{products:this.search(args)}};
  if(name==="get_product")return this.product(args);
  if(name==="compare_products")return {structuredContent:{products:products.filter(product=>(args.product_ids as string[]).includes(product.id))}};
  if(name==="set_cart")return this.setCart(args);
  if(name==="start_checkout")return this.startCheckout();
  return {isError:true,structuredContent:{reason:`the preview server has no tool ${name}`}};
 }
 private search(args:Json):Product[] {
  const query=String(args.query??"").toLowerCase();
  const store=args.store_id===undefined?undefined:stores.find(candidate=>candidate.id===args.store_id)?.name;
  return products.filter(product=>product.title.toLowerCase().includes(query)).filter(product=>store===undefined||product.merchant===store);
 }
 private product(args:Json):Reply {
  const found=this.find(args.product_id);
  return found?{structuredContent:found as unknown as Json}:{isError:true,structuredContent:{reason:"no such product"}};
 }
 private setCart(args:Json):Reply {
  const found=this.find(args.listing_id);
  if(!found)return {isError:true,structuredContent:{reason:"no such product"}};
  this.cart={listing_id:found.id,variant_id:found.variant_id,quantity:Number(args.quantity)};
  this.order="none";
  return {structuredContent:this.cartView()};
 }
 private startCheckout():Reply {
  if(!this.cart)return {isError:true,structuredContent:{reason:"the cart is empty"}};
  return {structuredContent:{url:`${checkout}?listing=${this.cart.listing_id}&variant=${this.cart.variant_id}&quantity=${this.cart.quantity}`}};
 }
}
