import {App,applyDocumentTheme,applyHostStyleVariables,type McpUiHostContext} from "@modelcontextprotocol/ext-apps";
import {OpenAIExtensions} from "@openai/mcp-extensions/app";
import {mount,type Extensions,type Host,type HostContext,type Page} from "./widget.ts";
const app=new App({name:"Vadora",version:"0.1.0"},{availableDisplayModes:["inline","fullscreen"]});
const extensions=new OpenAIExtensions(app);
// The host's theme and style variables give the page the colors and the font of the conversation around it (011:80).
function styleHost(context:HostContext|undefined):void {
 const host=context as McpUiHostContext|undefined;
 if(host?.theme)applyDocumentTheme(host.theme);
 if(host?.styles?.variables)applyHostStyleVariables(host.styles.variables);
}
mount(app as unknown as Host,extensions as unknown as Extensions,{document,window:window as unknown as Page["window"],styleHost});
