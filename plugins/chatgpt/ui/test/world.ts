import assert from "node:assert/strict";
import {mount} from "../src/widget.ts";
import {settle,SimExtensions,SimHost,SimPage,type FakeElement,type Handler} from "./sim.ts";

// These are the fixtures and the helpers that the widget tests share: a catalog of two products, a cart and an order, and
// a function that opens the widget in a simulated host and has the host deliver the result of the tool call that opened it.

export const kettle={id:"lst_kettle",variant_id:"lst_kettle",title:"TEST ceramic kettle",merchant:"TEST store",price:"5.00",currency:"USD",available:true};
export const scarf={id:"lst_scarf",variant_id:"lst_scarf",title:"TEST wool scarf",merchant:"TEST store",price:"25.00",currency:"USD",available:false};
export const checkout="https://checkout.vadora.xyz/pay?listing=lst_kettle&variant=lst_kettle&quantity=2";
export const cartOf=(quantity:number)=>({listing_id:"lst_kettle",variant_id:"lst_kettle",quantity});
export const orderOf=(payment:string)=>({number:"ORD-7",total:"11.00 USDC",payment_status:payment,fulfillment_status:payment==="paid"?"received":"awaiting_payment",
 receipt_url:"https://checkout.vadora.xyz/orders/ord_7"});
export const marketplace={products:[kettle,scarf],stores:[{id:"s1",name:"TEST store",url:"https://vadora.xyz/stores/s1"}],settings:{mode:"shop"}};

// open mounts the widget in a simulated host, has the host deliver the result of the tool call that opened it, and lets
// everything that the widget started finish. A test that does not say otherwise runs the fullscreen view, which has every
// control. The inline view is the small card of 011:80, and its tests ask for it.
export async function open(initial:unknown,server:Record<string,Handler>={},mode:"inline"|"fullscreen"="fullscreen") {
 const host=new SimHost();host.server=server;host.context={availableDisplayModes:["inline","fullscreen"],displayMode:mode};
 const extensions=new SimExtensions();
 const page=new SimPage();
 mount(host,extensions,page.asPage());
 await settle();
 host.deliver(initial);
 await settle();
 return {host,extensions,page,root:page.root};
}
export const result=(structuredContent:unknown)=>({structuredContent});
export const one=(buttons:FakeElement[]):FakeElement=>{assert.equal(buttons.length,1,"exactly one such button");return buttons[0];};

// The helpers of the inline view tests count what a person sees.
export const cardsOf=(root:FakeElement)=>root.all().filter(node=>node.tagName==="article");
export const labelsOf=(root:FakeElement)=>root.all().filter(node=>node.tagName==="button").map(node=>node.textContent);
export const imagesOf=(root:FakeElement)=>root.all().filter(node=>node.tagName==="img");
export const itemOf=(index:number)=>({...kettle,id:`lst_${index}`,variant_id:`lst_${index}`,title:`TEST item ${index}`,images:[`https://cdn.shopify.com/a/${index}.png`]});
export const items=(count:number)=>Array.from({length:count},(_,index)=>itemOf(index+1));
