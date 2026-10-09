import {test} from "node:test";
import assert from "node:assert/strict";
import {allowedImage,deepLink,checkoutLink,context} from "../src/state.ts";
test("navigation accepts only the documented routes and rejects double decoding",()=>{
 assert.deepEqual(deepLink("/products/a"),{route:"/products",ids:["a"]});
 for(const input of ["//evil.test/x","/products/a?token=x","/products/%252f","/compare?ids=a,b,c,d,e","/cart#x","/../secret"])
 assert.deepEqual(deepLink(input),{route:"/",ids:[]});
 assert.deepEqual(deepLink("/compare?ids=a,b"),{route:"/compare",ids:["a","b"]});
});
test("images require an exact allowed domain and raster suffix",()=>{
 assert.equal(allowedImage("https://cdn.shopify.com/a.webp?v=1"),true);
 for(const image of ["https://cdn.shopify.com.evil.test/a.jpg","https://u:p@cdn.shopify.com/a.jpg","https://cdn.shopify.com/a.svg","data:image/png,x","https://cdn.shopify.com:444/a.jpg"])
 assert.equal(allowedImage(image),false);
});
test("payment navigation allows the canonical handoff and no credential",()=>{
 assert.ok(checkoutLink("https://checkout.vadora.xyz/pay?listing=a&variant=b&quantity=2"));
 for(const link of ["https://checkout.vadora.xyz.evil.test/pay?listing=a&variant=b&quantity=2","https://checkout.vadora.xyz/pay?listing=a&variant=b&quantity=0","https://checkout.vadora.xyz/pay?listing=a&variant=b&quantity=2&token=x","https://checkout.vadora.xyz/pay?listing=a&listing=c&quantity=2","javascript:alert(1)"])
 assert.equal(checkoutLink(link),undefined);
});
test("model context contains only the permitted selection and status fields",()=>{
 const data=context({cart:{listing_id:"a",variant_id:"b",quantity:1},order:{number:"1",total:"2 USDC",payment_status:"paid",fulfillment_status:"received",receipt_url:"https://secret"}},["a"]);
 assert.deepEqual(data,{comparison:["a"],cart:{listing_id:"a",variant_id:"b",quantity:1},order:{number:"1",total:"2 USDC",payment_status:"paid",fulfillment_status:"received"}});
});
