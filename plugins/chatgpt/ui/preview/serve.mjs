// serve.mjs runs the widget preview on this machine. It listens on the loopback address only, answers only a request whose host
// header names that address, serves the host page, the real widget files from the assets folder, and the host script that it
// builds with esbuild, and it reads no other file.
import http from "node:http";
import {readFile} from "node:fs/promises";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {build} from "esbuild";

const here=path.dirname(fileURLToPath(import.meta.url));
const assets=path.resolve(here,"../../assets");

async function hostScript() {
 const result=await build({entryPoints:[path.join(here,"host.ts")],bundle:true,format:"esm",target:"es2022",write:false,logLevel:"silent"});
 return result.outputFiles[0].text;
}

// routes holds every path that the preview serves, so no other path reaches the disk. The origin is the one that the request
// named, so the widget's page loads its own files from the preview.
const routes={
 "/":async()=>({type:"text/html; charset=utf-8",body:await readFile(path.join(here,"host.html"))}),
 "/host.js":async()=>({type:"text/javascript; charset=utf-8",body:await hostScript()}),
 "/frame.html":async origin=>({type:"text/html; charset=utf-8",body:(await readFile(path.join(assets,"widget.html"),"utf8")).replaceAll("{{ASSET_ORIGIN}}",origin)}),
 "/chatgpt/widget.js":async()=>({type:"text/javascript; charset=utf-8",body:await readFile(path.join(assets,"widget.js")),shared:true}),
 "/chatgpt/widget.css":async()=>({type:"text/css; charset=utf-8",body:await readFile(path.join(assets,"widget.css")),shared:true}),
};

// sharedHeaders are what the assets origin sends with the widget files. A host runs the widget in a sandboxed frame whose origin
// is null, and a browser fetches a module script in CORS mode, so the script loads only when the response names every origin.
const sharedHeaders={"access-control-allow-origin":"*","cross-origin-resource-policy":"cross-origin"};

function refuse(response,status,message) {
 response.writeHead(status,{"content-type":"text/plain"});
 response.end(message);
}

// startPreview listens on the loopback address and resolves with the server and its origin. A port of 0 takes a free port.
export function startPreview(port=4173) {
 const server=http.createServer(async(request,response)=>{
  const allowed=[`127.0.0.1:${server.address().port}`,`localhost:${server.address().port}`];
  if(!allowed.includes(request.headers.host??"")) {refuse(response,403,"forbidden");return;}
  const origin=`http://${request.headers.host}`;
  const route=routes[new URL(request.url??"/",origin).pathname];
  if(request.method!=="GET"||!route) {refuse(response,404,"not found");return;}
  try {
   const {type,body,shared}=await route(origin);
   response.writeHead(200,{"content-type":type,"cache-control":"no-store",...(shared?sharedHeaders:{})});
   response.end(body);
  } catch(error) {
   refuse(response,500,String(error));
  }
 });
 return new Promise(resolve=>{
  server.listen(port,"127.0.0.1",()=>{resolve({server,origin:`http://127.0.0.1:${server.address().port}`});});
 });
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  const {origin}=await startPreview(Number(process.env.PORT??4173));
  console.log(`Widget preview: ${origin}/`);
}
