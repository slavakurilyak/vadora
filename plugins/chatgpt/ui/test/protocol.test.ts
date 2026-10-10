import {test} from "node:test";
import assert from "node:assert/strict";
import {App} from "@modelcontextprotocol/ext-apps";
import {AppBridge} from "@modelcontextprotocol/ext-apps/app-bridge";
// The MCP SDK is installed as the peer of the MCP Apps package, and its in memory transport joins the two sides below.
import {InMemoryTransport} from "@modelcontextprotocol/sdk/inMemory.js";
import {OpenAIExtensions} from "@openai/mcp-extensions/app";
import {mount,type Extensions,type Host} from "../src/widget.ts";
import {click,SimPage,type FakeElement} from "./sim.ts";

// These tests run the widget over the real MCP Apps protocol. The app side is the real App and the real OpenAIExtensions
// of the browser entry, and the host side is the real AppBridge that a host such as ChatGPT embeds, joined by an in memory
// transport. The host answers tool calls, link requests, display mode requests, model context updates, and messages the way
// a host does, so the requests that the widget sends are checked against the protocol's own schemas (011:106).

const kettle={id:"lst_kettle",variant_id:"lst_kettle",title:"TEST ceramic kettle",merchant:"TEST store",price:"5.00",currency:"USD",available:true,images:["https://cdn.shopify.com/a/kettle.png"]};
const items=(count:number)=>Array.from({length:count},(_,index)=>({...kettle,id:`lst_${index+1}`,variant_id:`lst_${index+1}`,title:`TEST item ${index+1}`}));
const checkout="https://checkout.vadora.xyz/pay?listing=lst_kettle&variant=lst_kettle&quantity=1";
const labelsOf=(root:FakeElement)=>root.all().filter(node=>node.tagName==="button").map(node=>node.textContent);
const cardsOf=(root:FakeElement)=>root.all().filter(node=>node.tagName==="article");

type Reply={isError?:boolean;structuredContent?:Record<string,unknown>};
type Options={context?:Record<string,unknown>;announces?:boolean;server?:Record<string,(args:Record<string,unknown>)=>Reply>};

// quiet lets every message that is in flight between the two sides arrive.
async function quiet():Promise<void> {for(let turn=0;turn<24;turn++)await new Promise<void>(resolve=>setTimeout(resolve,0));}

// connect joins the widget and a host. The host listens first, the widget connects through the real App, and the host's
// initialization answer carries the host context and the host capabilities, which the widget reads before it draws.
async function connect(options:Options={}) {
 const [appSide,hostSide]=InMemoryTransport.createLinkedPair();
 const app=new App({name:"Vadora",version:"0.1.0"},{availableDisplayModes:["inline","fullscreen"]},{autoResize:false});
 const extensions=new OpenAIExtensions(app);
 const hostContext={displayMode:"inline",availableDisplayModes:["inline","fullscreen"],...options.context};
 const bridge=new AppBridge(null,{name:"test host",version:"1.0.0"},{
  serverTools:{},openLinks:{},updateModelContext:{text:{}},message:{text:{}},
  experimental:{"openai/message":{},"openai/modelContext":{}},
 },{hostContext:hostContext as never});
 const seen={calls:[] as Array<{name:string;args:Record<string,unknown>}>,opened:[] as string[],messages:[] as unknown[],updates:[] as Array<{content?:unknown;structuredContent?:unknown}>,modes:[] as string[]};
 let counter=0;
 bridge.oncalltool=async params=>{
  const args=(params.arguments??{}) as Record<string,unknown>;
  seen.calls.push({name:params.name,args});
  const handler=options.server?.[params.name];
  if(!handler)return {content:[],isError:true,structuredContent:{reason:"the server has no such tool"}};
  return {content:[],...handler(args)};
 };
 bridge.onopenlink=async({url})=>{seen.opened.push(url);return {};};
 bridge.onmessage=async params=>{seen.messages.push(params);return {};};
 bridge.onrequestdisplaymode=async({mode})=>{
  seen.modes.push(mode);
  const granted=(hostContext.availableDisplayModes as string[]).includes(mode)?mode:"inline";
  if(options.announces!==false)await bridge.sendHostContextChange({displayMode:granted} as never);
  return {mode:granted} as never;
 };
 // The host keeps the update that the widget sent as its current one, and tells the widget so, as ChatGPT does.
 bridge.onupdatemodelcontext=async params=>{
  seen.updates.push(params as {content?:unknown;structuredContent?:unknown});
  counter++;
  const updateId=`update-${counter}`;
  setTimeout(()=>{void bridge.sendHostContextChange({"openai/modelContext":{updateId}} as never);},0);
  return {_meta:{"openai/modelContext":{updateId}}} as never;
 };
 const initialized=new Promise<void>(resolve=>{bridge.oninitialized=()=>resolve();});
 await bridge.connect(hostSide);
 const connectApp=app.connect.bind(app);
 (app as unknown as {connect:(transport?:unknown)=>Promise<void>}).connect=(transport?:unknown)=>connectApp((transport??appSide) as never);
 const page=new SimPage();
 mount(app as unknown as Host,extensions as unknown as Extensions,page.asPage());
 await initialized;
 await quiet();
 return {
  bridge,page,seen,root:page.root,
  // deliver is the host passing the widget the result of the tool call that opened it.
  deliver:async(structuredContent:Record<string,unknown>)=>{await bridge.sendToolResult({content:[],structuredContent} as never);await quiet();},
  // report is the host telling the widget that its context changed, as when the person closes the fullscreen view.
  report:async(changed:Record<string,unknown>)=>{await bridge.sendHostContextChange(changed as never);await quiet();},
  press:async(button:FakeElement|undefined)=>{await click(button);await quiet();},
 };
}

test("over the real bridge, Select calls set_cart, the model context carries the selection, and Pay with USDC opens the checkout link",async()=>{
 const world=await connect({server:{
  set_cart:args=>({structuredContent:{cart:{listing_id:args.listing_id,variant_id:args.variant_id,quantity:args.quantity},product:items(3).find(item=>item.id===args.listing_id),checkout_enabled:true}}),
  start_checkout:()=>({structuredContent:{url:checkout}}),
 }});
 await world.deliver({products:items(3),settings:{mode:"shop"}});
 assert.equal(cardsOf(world.root).length,3,"the first tool result draws the card with no second tool call");
 assert.deepEqual(world.seen.calls,[]);
 assert.deepEqual(labelsOf(world.root),["Select","Select","Select","Expand"]);

 await world.press(cardsOf(world.root)[0].buttons("Select")[0]);
 assert.deepEqual(world.seen.calls,[{name:"set_cart",args:{listing_id:"lst_1",variant_id:"lst_1",quantity:1}}]);
 assert.deepEqual(labelsOf(world.root),["Pay with USDC","Refresh order status"]);
 assert.equal(world.seen.updates.length,1,"one update of the model context, and the host's answer starts no loop");
 assert.deepEqual(world.seen.updates[0].structuredContent,{comparison:[],cart:{listing_id:"lst_1",variant_id:"lst_1",quantity:1}});
 const blocks=JSON.stringify(world.seen.updates[0].content);
 assert.ok(blocks.includes("Selected product: TEST item 1"));
 for(const word of ["token","delivery","address","bearer"])assert.ok(!blocks.toLowerCase().includes(word),word);

 await world.press(world.root.buttons("Pay with USDC")[0]);
 assert.deepEqual(world.seen.calls.map(call=>call.name),["set_cart","start_checkout"]);
 assert.deepEqual(world.seen.opened,[checkout],"the link goes through the host's own open link request");
 assert.equal(world.seen.updates.length,1);
});

test("over the real bridge, Expand asks for fullscreen, and the host's report that the person closed it draws the card again",async()=>{
 const world=await connect();
 await world.deliver({products:items(10),settings:{mode:"shop"},stores:[{id:"s1",name:"TEST store",url:"https://vadora.xyz/stores/s1"}]});
 assert.equal(cardsOf(world.root).length,8);
 await world.press(world.root.buttons("Expand")[0]);
 assert.deepEqual(world.seen.modes,["fullscreen"]);
 assert.equal(cardsOf(world.root).length,10);
 for(const label of ["Shop","Cart","Sell","Search","TEST store"])assert.equal(world.root.buttons(label).length,1,label);
 await world.report({displayMode:"inline"});
 assert.equal(cardsOf(world.root).length,8);
 assert.equal(world.root.buttons("Shop").length,0);
 assert.deepEqual(world.seen.calls,[],"a change of display mode reads nothing from the server");
});

test("over the real bridge, a host that answers the display mode request without announcing it still gets the fullscreen view",async()=>{
 const world=await connect({announces:false});
 await world.deliver({products:items(4),settings:{mode:"shop"}});
 await world.press(world.root.buttons("Expand")[0]);
 assert.deepEqual(world.seen.modes,["fullscreen"]);
 assert.equal(world.root.buttons("Compare").length,4);
 await world.report({displayMode:"inline"});
 assert.equal(world.root.buttons("Compare").length,0);
});

test("over the real bridge, a host that offers no fullscreen gets no Expand, and one that starts in fullscreen draws the whole marketplace",async()=>{
 const inlineOnly=await connect({context:{availableDisplayModes:["inline"]}});
 await inlineOnly.deliver({products:items(3),settings:{mode:"shop"}});
 assert.deepEqual(labelsOf(inlineOnly.root),["Select","Select","Select"]);

 const fullscreen=await connect({context:{displayMode:"fullscreen"}});
 await fullscreen.deliver({products:items(3),settings:{mode:"shop"}});
 for(const label of ["Shop","Cart","Sell","Search"])assert.equal(fullscreen.root.buttons(label).length,1,label);
 assert.equal(fullscreen.root.buttons("Compare").length,3);
});

test("over the real bridge, Ask about this product reaches the host as a user message with the product and its merchant",async()=>{
 const world=await connect({context:{displayMode:"fullscreen"}});
 await world.deliver({products:items(2),settings:{mode:"shop"}});
 await world.press(cardsOf(world.root)[0].buttons("Ask about this product")[0]);
 assert.equal(world.seen.messages.length,1);
 const message=world.seen.messages[0] as {role:string;content:unknown};
 assert.equal(message.role,"user");
 assert.deepEqual(message.content,[{type:"text",text:"Tell me more about TEST item 1 from TEST store."}]);
});

test("over the real bridge, the deep link in the host context opens that route, and a hostile one opens the root",async()=>{
 const product=await connect({context:{"openai/deepLink":{url:"/products/lst_kettle"}},server:{get_product:()=>({structuredContent:kettle})}});
 assert.deepEqual(product.seen.calls,[{name:"get_product",args:{product_id:"lst_kettle"}}]);
 assert.ok(product.root.text().includes("TEST ceramic kettle"));

 for(const hostile of ["//evil.test/x","/cart#x","/products/a?token=x"]) {
  const world=await connect({context:{"openai/deepLink":{url:hostile}},server:{open_cart:()=>({structuredContent:{}}),get_product:()=>({structuredContent:kettle})}});
  assert.deepEqual(world.seen.calls,[],hostile);
  assert.deepEqual(world.seen.opened,[],hostile);
 }
});

test("over the real bridge, a server error that names the workspace opens the workspace through the host and nothing else",async()=>{
 const world=await connect({context:{displayMode:"fullscreen"},server:{
  search_products:()=>({isError:true,structuredContent:{reason:"confirm in the workspace",workspace_url:"https://app.vadora.xyz/account/agreements"}}),
 }});
 await world.deliver({products:items(2),settings:{mode:"shop"}});
 await world.press(world.root.buttons("Search")[0]);
 assert.deepEqual(world.seen.opened,["https://app.vadora.xyz/account/agreements"]);
 assert.ok(world.root.text().includes("Request failed. Refresh the view and try again."));
});

test("over the real bridge, the page takes the host's theme and style variables at connection and at each change",async()=>{
 const world=await connect({context:{theme:"dark",styles:{variables:{"--color-text-primary":"#ffffff"}}}});
 const first=world.page.styled[0] as {theme?:string;styles?:{variables?:Record<string,string>}}|undefined;
 assert.equal(first?.theme,"dark");
 assert.equal(first?.styles?.variables?.["--color-text-primary"],"#ffffff");
 await world.report({theme:"light"});
 const last=world.page.styled.at(-1) as {theme?:string;styles?:{variables?:Record<string,string>}}|undefined;
 assert.equal(last?.theme,"light");
 assert.equal(last?.styles?.variables?.["--color-text-primary"],"#ffffff","the host context keeps what the change did not name");
});

test("over the real bridge, a change of the model context that the widget did not make makes it read the server, and its own update does not",async()=>{
 const state:Record<string,unknown>={cart:{listing_id:"lst_kettle",variant_id:"lst_kettle",quantity:1},product:kettle,checkout_enabled:true};
 const world=await connect({server:{open_cart:()=>({structuredContent:state})}});
 await world.deliver({...state});
 assert.deepEqual(world.seen.calls,[]);
 await world.report({"openai/modelContext":{updateId:"made-by-the-model"}});
 assert.deepEqual(world.seen.calls.map(call=>call.name),["open_cart"]);
 assert.equal(world.seen.updates.length,1,"the reread publishes the model context once");
 await quiet();
 assert.deepEqual(world.seen.calls.map(call=>call.name),["open_cart"],"the host's report of the widget's own update starts no loop");
});
