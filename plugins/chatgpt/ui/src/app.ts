import {App} from "@modelcontextprotocol/ext-apps";
import {OpenAIExtensions} from "@openai/mcp-extensions/app";
import {mount,type Extensions,type Host} from "./widget.ts";
const app=new App({name:"Vadora",version:"0.1.0"},{availableDisplayModes:["inline","fullscreen"]});
const extensions=new OpenAIExtensions(app);
mount(app as unknown as Host,extensions as unknown as Extensions,{document,window:window as unknown as import("./widget.ts").Page["window"]});
