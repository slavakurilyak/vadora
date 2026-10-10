import {test} from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import {readFileSync} from "node:fs";
import {startPreview} from "../preview/serve.mjs";
import {products,World} from "../preview/world.ts";
import {allowedImage,checkoutLink,receiptLink} from "../src/state.ts";

// These tests cover the widget preview: a small server on the loopback address that hosts the real widget for a person to see in
// a browser. The preview opens nothing to the network, reads only the files that it names, and serves data that passes the
// widget's own image and link rules.

const asset=(name:string)=>readFileSync(new URL(`../../assets/${name}`,import.meta.url));

async function get(origin:string,pathname:string,init?:RequestInit) {
 const response=await fetch(origin+pathname,init);
 return {status:response.status,body:Buffer.from(await response.arrayBuffer()),headers:response.headers};
}

test("the preview listens on the loopback address and serves the host page, the widget files, and nothing else",async()=>{
 const {server,origin}=await startPreview(0);
 try {
  assert.equal((server.address() as {address:string}).address,"127.0.0.1");
  const page=await get(origin,"/");
  assert.equal(page.status,200);
  assert.ok(page.body.toString().includes("<title>Widget preview</title>"));
  const script=await get(origin,"/host.js");
  assert.equal(script.status,200);
  assert.ok(script.body.toString().includes("AppBridge"));
  const frame=await get(origin,"/frame.html");
  assert.equal(frame.status,200);
  const html=frame.body.toString();
  assert.ok(!html.includes("{{"),"the placeholder for the asset origin is filled");
  assert.ok(html.includes(`${origin}/chatgpt/widget.js`));
  assert.ok(html.includes(`${origin}/chatgpt/widget.css`));
  assert.deepEqual((await get(origin,"/chatgpt/widget.js")).body,asset("widget.js"));
  assert.deepEqual((await get(origin,"/chatgpt/widget.css")).body,asset("widget.css"));
  for(const other of ["/package.json","/preview/serve.mjs","/../package.json","/%2e%2e/package.json","/node_modules/esbuild/package.json","/chatgpt/widget.html","/chatgpt/../../package.json"]) {
   assert.equal((await get(origin,other)).status,404,other);
  }
  assert.equal((await get(origin,"/",{method:"POST"})).status,404);
 } finally {
  server.closeAllConnections();
  server.close();
 }
});

test("the preview serves the widget files to a frame of another origin, as the assets origin does, and no other file",async()=>{
 const {server,origin}=await startPreview(0);
 try {
  for(const file of ["/chatgpt/widget.js","/chatgpt/widget.css"]) {
   const answer=await get(origin,file);
   assert.equal(answer.headers.get("access-control-allow-origin"),"*",`${file} names every origin, so a sandboxed frame with the null origin loads it`);
   assert.equal(answer.headers.get("cross-origin-resource-policy"),"cross-origin");
  }
  for(const file of ["/","/host.js","/frame.html"]) {
   assert.equal((await get(origin,file)).headers.get("access-control-allow-origin"),null,`${file} opens itself to no other origin`);
  }
 } finally {
  server.closeAllConnections();
  server.close();
 }
});

test("the preview refuses a request that names another host, so a web page cannot reach it through a rebound name",async()=>{
 const {server}=await startPreview(0);
 try {
  const port=(server.address() as {port:number}).port;
  const status=await new Promise<number>((resolve,reject)=>{
   const request=http.request({host:"127.0.0.1",port,path:"/",headers:{host:"evil.test"}},response=>{response.resume();resolve(response.statusCode??0);});
   request.on("error",reject);
   request.end();
  });
  assert.equal(status,403);
 } finally {
  server.closeAllConnections();
  server.close();
 }
});

test("the preview data passes the image and link rules of the widget, and every product is labeled as a test",()=>{
 for(const product of products) {
  assert.ok(product.images.every(allowedImage),`${product.title} has images that the widget accepts`);
  assert.ok(product.title.startsWith("TEST "),`${product.title} is labeled TEST`);
 }
 const world=new World("cart","paid");
 const cart=world.call("open_cart",{}).structuredContent as {order:{receipt_url:string};cart:{quantity:number}};
 assert.ok(receiptLink(cart.order.receipt_url),"the receipt link is one that the widget opens");
 const link=(world.call("start_checkout",{}).structuredContent as {url:string}).url;
 assert.ok(checkoutLink(link),"the checkout link is one that the widget opens");
});

test("the preview server answers a cart, a search, and an error the way the real server shapes them",()=>{
 const world=new World("shop","none");
 assert.equal(world.call("start_checkout",{}).isError,true,"an empty cart has no checkout");
 const set=world.call("set_cart",{listing_id:products[0].id,variant_id:products[0].variant_id,quantity:3}).structuredContent as {cart:{quantity:number};checkout_enabled:boolean};
 assert.equal(set.cart.quantity,3);
 assert.equal(set.checkout_enabled,true);
 const found=world.call("search_products",{query:"kettle"}).structuredContent as {products:Array<{title:string}>};
 assert.deepEqual(found.products.map(product=>product.title),["TEST Ceramic kettle"]);
 assert.equal(world.call("get_product",{product_id:"missing"}).isError,true);
 assert.equal(world.call("not_a_tool",{}).isError,true);
 const pending=new World("cart","pending").call("open_cart",{}).structuredContent as {checkout_enabled:boolean;message:string};
 assert.equal(pending.checkout_enabled,false,"a pending payment asks for no second payment");
 assert.ok(pending.message.includes("Pay no second time"));
});
