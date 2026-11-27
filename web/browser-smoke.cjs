const {chromium}=require('playwright');
const http=require('node:http');
const fs=require('node:fs');
const assert=require('node:assert/strict');
const html=fs.readFileSync('web/index.html','utf8');
const provider={name:'spotify-domain',type:'http',url:'https://example.org/spotify-domain.mrs',behavior:'domain',format:'mrs',path:'./ruleset/spotify-domain.mrs'};
let installed=true, routed=true;
const local=()=>({mode:'nikki',config_path:'/etc/nikki/mixin.yaml',rules_dir:'/etc/nikki/run/ruleset',rules:[...(routed?['RULE-SET,spotify-domain,VPN-NON-RU']:[]),'MATCH,DIRECT'],rule_providers:installed?[provider]:[],proxy_group_entries:[{name:'VPN-NON-RU',type:'url-test',include_all_proxies:true}],proxy_groups:['VPN-NON-RU','DIRECT','REJECT'],routing_targets:['VPN-NON-RU','DIRECT','REJECT'],proxy_providers:[],disk_files:[{name:'spotify-domain.mrs',size:123}],mihomo_status:{alive:true,version:'v1.19.32'}});
const catalog=()=>({items:[{...provider,category:'tachyon',path:'spotify-domain.mrs',registered:installed,provider_name:installed?provider.name:'',applied:routed,applied_name:routed?provider.name:'',downloaded:true,rule_count:25}],count:1});
const server=http.createServer((req,res)=>{
 const p=new URL(req.url,'http://localhost').pathname;
 if(p==='/'){res.setHeader('Content-Type','text/html');return res.end(html);}
 res.setHeader('Content-Type','application/json');
 if(p==='/api/local')return res.end(JSON.stringify(local()));
 if(p==='/api/catalog')return res.end(JSON.stringify(catalog()));
 if(p==='/api/mihomo/status')return res.end(JSON.stringify(local().mihomo_status));
 if(p==='/api/raw-rules'&&req.method==='DELETE'){routed=false;return res.end('{"status":"ok"}');}
 if(p==='/api/rules'&&(req.method==='PUT'||req.method==='POST')){installed=true;routed=true;return res.end('{"status":"ok"}');}
 if(p==='/api/rules'&&req.method==='DELETE'){installed=false;routed=false;return res.end('{"status":"ok"}');}
 res.statusCode=404;res.end('{"error":"unexpected request"}');
});
(async()=>{
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 let browser;
 try{
 browser=await chromium.launch({headless:true,args:['--no-sandbox']});
 const page=await browser.newPage({viewport:{width:1440,height:900}});
 const errors=[];const external=[];page.on('pageerror',e=>errors.push(e.message));
 const base='http://127.0.0.1:'+server.address().port;
 await page.route('**/*',route=>{if(route.request().url().startsWith(base))return route.continue();external.push(route.request().url());return route.abort();});
 await page.goto(base);
 await page.getByRole('heading',{name:'Routes',exact:true}).waitFor();
 await page.locator('tbody').getByText('spotify-domain',{exact:true}).waitFor();
 await page.getByRole('button',{name:'Remove route',exact:true}).first().click();
 await page.locator('#modal-del-raw-confirm').click();
 await page.locator('#modal-container').waitFor({state:'hidden'});
 await page.getByRole('button',{name:/^Rulesets/}).click();
 await page.locator('tbody').getByText('Not routed',{exact:true}).waitFor();
 await page.getByRole('button',{name:'Browse catalog',exact:true}).click();
 await page.locator('#cat-tbody').getByText('Installed · unused',{exact:true}).waitFor();
 await page.locator('#cat-tbody').getByRole('button',{name:'Route',exact:true}).click();
 await page.locator('#modal-group').selectOption('VPN-NON-RU');
 await page.locator('#modal-submit-btn').click();
 await page.locator('#cat-tbody').getByText('Routed',{exact:true}).waitFor();
 await page.getByRole('button',{name:/^Proxy groups/}).click();
 await page.getByText('All subscription nodes',{exact:true}).waitFor();
 await page.getByRole('button',{name:'Advanced',exact:true}).click();
 await page.locator('#btn-toggle-quic').waitFor();
 assert.deepEqual(errors,[]);assert.deepEqual(external,[]);
 console.log('Browser navigation, route removal and re-routing passed');
 }finally{if(browser)await browser.close();server.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
