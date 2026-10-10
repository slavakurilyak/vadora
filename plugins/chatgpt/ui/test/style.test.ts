import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";

// These tests read the stylesheet and the page shell that the widget ships, and check the visual rules of 011:80 that a
// reader can verify from the files: text contrast at WCAG AA on both fallback palettes, sizes that follow the reader's text
// size, no second scroll area, the system font, and the brand color on primary buttons only.
const css=readFileSync(new URL("../../assets/widget.css",import.meta.url),"utf8").replace(/\/\*[\s\S]*?\*\//g,"");
const html=readFileSync(new URL("../../assets/widget.html",import.meta.url),"utf8");

type Rule={selector:string;declarations:Map<string,string>};

function declarations(body:string):Map<string,string> {
 const map=new Map<string,string>();
 for(const part of body.split(";")) {
  const colon=part.indexOf(":");
  if(colon>=0)map.set(part.slice(0,colon).trim(),part.slice(colon+1).trim());
 }
 return map;
}

type Block={selector:string;body:string};

// blocks splits a stylesheet into its top level blocks by matching braces.
function blocks(source:string):Block[] {
 const found:Block[]=[];
 let index=0;
 while(index<source.length) {
  const open=source.indexOf("{",index);
  if(open<0)break;
  let depth=1;
  let end=open+1;
  while(depth>0&&end<source.length) {
   if(source[end]==="{")depth++;
   if(source[end]==="}")depth--;
   end++;
  }
  found.push({selector:source.slice(index,open).trim(),body:source.slice(open+1,end-1)});
  index=end;
 }
 return found;
}

// rules reads the rules of the stylesheet. A rule inside a media block carries the block's condition in its selector, and the
// media blocks of this stylesheet hold rules only.
function rules(source:string):Rule[] {
 const found:Rule[]=[];
 for(const block of blocks(source)) {
  if(!block.selector.startsWith("@media")) {
   found.push({selector:block.selector,declarations:declarations(block.body)});
   continue;
  }
  for(const inner of blocks(block.body))found.push({selector:`${block.selector} ${inner.selector}`,declarations:declarations(inner.body)});
 }
 return found;
}

const all=rules(css);

// A page is drawn in one of three palettes: the light one, the dark one that the reader's preference turns on, and the dark
// one that the host's dark theme turns on. tokens are the custom properties that apply to the page in one of them.
type Scheme="light"|"preference"|"theme";
const schemes:Scheme[]=["light","preference","theme"];
function tokens(scheme:Scheme):Map<string,string> {
 const map=new Map<string,string>();
 for(const rule of all) {
  const applies=rule.selector===":root"
   ||(scheme==="preference"&&rule.selector.includes("prefers-color-scheme:dark"))
   ||(scheme==="theme"&&rule.selector===":root[data-theme=dark]");
  if(applies)for(const [name,value] of rule.declarations)if(name.startsWith("--"))map.set(name,value);
 }
 return map;
}

// resolve turns a value into the color it has when the host passes no variables: a variable of the host takes its fallback.
function resolve(value:string,palette:Map<string,string>):string {
 let current=value.trim();
 for(let depth=0;depth<8;depth++) {
  const match=/^var\(\s*(--[\w-]+)\s*(?:,\s*(.+))?\)$/.exec(current);
  if(!match)return current;
  const next=palette.get(match[1])??match[2];
  assert.ok(next!==undefined,`${match[1]} has no value and no fallback`);
  current=next.trim();
 }
 return assert.fail(`the value ${value} nests variables too deep`);
}

function channel(value:number):number {
 const scaled=value/255;
 return scaled<=0.03928?scaled/12.92:Math.pow((scaled+0.055)/1.055,2.4);
}
function luminance(color:string):number {
 assert.match(color,/^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i,`${color} is a hex color`);
 const digits=color.slice(1);
 const full=digits.length===3?[...digits].map(digit=>digit+digit).join(""):digits;
 const [red,green,blue]=[0,2,4].map(start=>parseInt(full.slice(start,start+2),16));
 return 0.2126*channel(red)+0.7152*channel(green)+0.0722*channel(blue);
}
function contrast(first:string,second:string):number {
 const [lighter,darker]=[luminance(first),luminance(second)].sort((a,b)=>b-a);
 return (lighter+0.05)/(darker+0.05);
}

function raw(selector:string,property:string):string {
 const rule=all.find(candidate=>candidate.selector.split(",").map(part=>part.trim()).includes(selector));
 assert.ok(rule,`the stylesheet has a rule for ${selector}`);
 const value=rule.declarations.get(property);
 assert.ok(value!==undefined,`${selector} declares ${property}`);
 return value;
}
function declared(selector:string,property:string,scheme:Scheme):string {return resolve(raw(selector,property),tokens(scheme));}
// borderColor is the color at the end of a border shorthand, resolved.
function borderColor(selector:string,scheme:Scheme):string {
 const last=/(var\([^)]*\)|#[0-9a-f]{3,6})$/i.exec(raw(selector,"border"));
 assert.ok(last,`${selector} ends its border with a color`);
 return resolve(last[1],tokens(scheme));
}

test("the contrast function reproduces the published WCAG ratios",()=>{
 assert.equal(Math.round(contrast("#000000","#ffffff")*100)/100,21);
 assert.ok(contrast("#767676","#ffffff")>=4.5,"#767676 on white is the lightest gray that passes AA");
 assert.ok(contrast("#777777","#ffffff")<4.5,"#777777 on white fails AA");
});

test("text keeps WCAG AA contrast on the light palette and on both dark palettes",()=>{
 const pairs:Array<[string,string,string,string,string]>=[
  ["page text",":root","color",":root","background"],
  ["muted text",".muted","color",":root","background"],
  ["button text","button","color","button","background"],
  ["primary button text","button.primary","color","button.primary","background"],
 ];
 for(const dark of ["preference","theme"] as const)assert.notEqual(tokens(dark).get("--surface"),tokens("light").get("--surface"),`the ${dark} palette differs from the light one`);
 for(const scheme of schemes)for(const [name,foreground,property,backgroundSelector,backgroundProperty] of pairs) {
  const ratio=contrast(declared(foreground,property,scheme),declared(backgroundSelector,backgroundProperty,scheme));
  assert.ok(ratio>=4.5,`${name} in the ${scheme} palette has a contrast of ${ratio.toFixed(2)}, below 4.5`);
 }
 for(const scheme of schemes) {
  const ratio=contrast(borderColor("input",scheme),declared(":root","background",scheme));
  assert.ok(ratio>=3,`a field's border in the ${scheme} palette has a contrast of ${ratio.toFixed(2)}, below 3`);
 }
});

test("every size follows the reader's text size, nothing has a fixed height, and nothing scrolls",()=>{
 assert.ok(!/\d\s*px\b/.test(css.replace(/\b1px solid\b/g,"")),"a size is in rem, and only a one pixel border is in pixels");
 assert.equal(declared(":root","font-size","light"),"1rem");
 assert.ok(!/(?:^|[;{\s])(?:max-|min-)?height\s*:/m.test(css),"no height, so enlarged text is never clipped");
 assert.ok(!/(?:^|[;{\s])overflow(?:-[xy])?\s*:/m.test(css),"no overflow rule, so a card holds no second scroll area");
 assert.ok(!/position\s*:\s*(?:fixed|sticky|absolute)/.test(css),"no layer that covers the conversation");
 assert.ok(!/(?:text-size-adjust|zoom)\s*:/.test(css),"no rule that blocks the reader's zoom");
 assert.ok(!/user-scalable\s*=\s*no|maximum-scale/i.test(html),"the viewport lets the reader scale the page");
 assert.match(html,/<meta name="viewport" content="width=device-width,initial-scale=1">/);
 assert.match(html,/<html lang="en">/);
});

test("the font is the host's or the system's, and the stylesheet loads nothing of its own",()=>{
 assert.match(declared(":root","font-family","light"),/system-ui/);
 assert.match(raw(":root","font-family"),/^var\(--font-sans,/);
 for(const forbidden of ["@font-face","@import","url("])assert.ok(!css.includes(forbidden),`the stylesheet has no ${forbidden}`);
});

test("the brand color marks the primary button and nothing else",()=>{
 assert.equal(css.split("#075e45").length-1,1,"the brand color is written once");
 const users=all.filter(rule=>[...rule.declarations.values()].some(value=>value.includes("var(--brand")));
 assert.deepEqual(users.map(rule=>rule.selector),["button.primary"]);
 const shell=html.toLowerCase();
 assert.ok(!shell.includes("<img")&&!shell.includes("logo"),"the page shell carries no logo");
 assert.match(html,/<main id="app" aria-live="polite">/);
});
