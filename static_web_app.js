(function(){
/* ── Multi-user: Ruby, Lynn, Raven (X-User header + localStorage + cookie for dashboard) ── */
function getCurrentUser(){ try{ var u=localStorage.getItem('locus_user'); if(u!=null)return u; }catch(e){} return ''; }
function getCurrentUserDisplayName(){ var u=getCurrentUser(); return u||''; }
function setLocusUserCookie(u){ try{ document.cookie='locus_user='+encodeURIComponent(u||'')+'; path=/; max-age=31536000'; }catch(e){} }
function apiHeaders(){ return {'X-User':getCurrentUser()}; }
function mergeApiHeaders(opts){ var h=opts&&opts.headers?Object.assign({},opts.headers):{}; Object.assign(h,apiHeaders()); return Object.assign({},opts,{headers:h}); }

/* ── State ── */
var mobileConvos=[],continueConvos=[],grokConvos=[],cursorConvos=[],archivedConvos=[];
var searchResults=null;
var MAX_IMAGE_ATTACHMENTS=1,MAX_FILE_ATTACHMENTS=5,MAX_TOTAL_ATTACHMENTS=6;
var pendingAttachments=[]; /* { type:'image'|'file', name, imageBase64?, fileText?, fileBase64?, fileMime? } */
function pendingImage(){return pendingAttachments.find(function(a){return a.type==='image';});}
function pendingFiles(){return pendingAttachments.filter(function(a){return a.type==='file';});}
function canAddImage(){return !pendingImage()&&pendingAttachments.length<MAX_TOTAL_ATTACHMENTS;}
function canAddFile(){return pendingFiles().length<MAX_FILE_ATTACHMENTS&&pendingAttachments.length<MAX_TOTAL_ATTACHMENTS;}
var currentId=null,currentSrc='mobile',currentRO=false;
var showArchive=false;
var currentMessages=[],currentConvoTitle='';
var currentBranchIndex=0,branchCount=1;
var currentFeedback={};
var CHAT_MODE_KEY='locus_chat_mode';
var currentMode=(function(){try{var s=localStorage.getItem(CHAT_MODE_KEY);if(s==='therapist'||s==='learning'||s==='bestie')return s;}catch(e){}return 'bestie';})();
var isGroupView=false;
var signedInUser=false;
/* ── Chat input draft (localStorage) ── */
var DRAFT_KEY='locus_chat_draft';
function getDrafts(){ try{ var s=localStorage.getItem(DRAFT_KEY); return s?JSON.parse(s):{}; }catch(e){ return {}; } }
function draftKey(id,src){ return (src||'mobile')+'_'+(id||''); }
function getDraft(id,src){ return getDrafts()[draftKey(id,src)]||''; }
function setDraft(id,src,text){ var o=getDrafts(); if(text)o[draftKey(id,src)]=text; else delete o[draftKey(id,src)]; try{ localStorage.setItem(DRAFT_KEY,JSON.stringify(o)); }catch(e){} }
function clearDraft(id,src){ setDraft(id,src,''); }
var _draftSaveTimer=null;
function saveDraftDebounced(){ if(currentSrc!=='mobile'||!currentId||!msgInput)return; var t=msgInput.value; clearTimeout(_draftSaveTimer); _draftSaveTimer=setTimeout(function(){ setDraft(currentId,currentSrc,t); _draftSaveTimer=null; },1000); }
var _draftRestoredHintEl=null;
function showDraftRestoredHint(){ if(_draftRestoredHintEl){ _draftRestoredHintEl.style.display='block'; clearTimeout(_draftRestoredHintEl._hideAt); } else { _draftRestoredHintEl=document.createElement('div'); _draftRestoredHintEl.setAttribute('role','status'); _draftRestoredHintEl.className='draft-restored-hint'; _draftRestoredHintEl.style.cssText='padding:8px 12px;margin:0 12px 8px;background:rgba(255,122,217,.15);border:1px solid rgba(255,122,217,.35);border-radius:10px;font-size:13px;color:var(--pink,#ff7ad9);'; _draftRestoredHintEl.textContent='Restored unsent message'; var inputArea=document.getElementById('inputArea'); if(inputArea&&inputArea.parentNode)inputArea.parentNode.insertBefore(_draftRestoredHintEl,inputArea); _draftRestoredHintEl.addEventListener('click',function(){ hideDraftRestoredHint(); }); } _draftRestoredHintEl._hideAt=setTimeout(function(){ hideDraftRestoredHint(); },5000); }
function hideDraftRestoredHint(){ if(_draftRestoredHintEl){ _draftRestoredHintEl.style.display='none'; if(_draftRestoredHintEl._hideAt)clearTimeout(_draftRestoredHintEl._hideAt); } }
function restoreDraftForConvo(id,src){ if(src!=='mobile'||!id||!msgInput)return; var text=getDraft(id,src); if(!text)return; msgInput.value=text; autoResize(); showDraftRestoredHint(); updateContextIndicator(); }

/* ── Elements ── */
var sidebar=document.getElementById('sidebar'),sideOverlay=document.getElementById('sideOverlay');
var sbList=document.getElementById('sbList'),sbSearch=document.getElementById('sbSearch'),sbNew=document.getElementById('sbNew');
var userSelect=document.getElementById('userSelect');
var avatarPicker=document.getElementById('avatarPicker');
var avatarCustomUrl=document.getElementById('avatarCustomUrl');
var avatarCustomBtn=document.getElementById('avatarCustomBtn');
var currentUserAvatarUrl='/user_avatar.svg';
var avatarCharacters=[];
var menuBtn=document.getElementById('menuBtn');
var chatArea=document.getElementById('chatArea'),typing=document.getElementById('typing'),generatingImage=document.getElementById('generatingImage');
var roHint=document.getElementById('roHint'),msgInput=document.getElementById('msgInput'),sendBtn=document.getElementById('sendBtn');
var pendingDoubleTextQueue=[],sendInProgress=false;
var imgFile=document.getElementById('imgFile'),fileInput=document.getElementById('fileInput'),attachImgBtn=document.getElementById('attachImgBtn');
var hdrName=document.getElementById('hdrName'),hdrSub=document.getElementById('hdrSub'),hdrAvatar=document.getElementById('hdrAvatar');
var hdrCopyExportWrap=document.getElementById('hdrCopyExportWrap'),hdrCopyExportBtn=document.getElementById('hdrCopyExportBtn'),hdrCopyExportDropdown=document.getElementById('hdrCopyExportDropdown');
var copyConvoAction=document.getElementById('copyConvoAction'),exportConvoAction=document.getElementById('exportConvoAction');
var forkConvoBtn=document.getElementById('forkConvoBtn');
var userProfileWrap=document.getElementById('userProfileWrap');
var userProfilePronouns=document.getElementById('userProfilePronouns');
var userProfileAbout=document.getElementById('userProfileAbout');
var userProfileSave=document.getElementById('userProfileSave');
var appEl=document.getElementById('app');
var tabBar=document.getElementById('tabBar');
var roomPanel=document.getElementById('roomPanel');
var locusSprite=document.getElementById('locusSprite');
var activityBubble=document.getElementById('activityBubble');
function showInitErr(msg){ try{ var div=document.createElement('div'); div.style.cssText='padding:16px;color:#ff7ad9;font-size:14px;white-space:pre-wrap;background:#1a0a1a;'; div.textContent=msg; var el=document.getElementById('chatArea'); (el||document.body).insertBefore(div,(el&&el.firstChild)||document.body.firstChild); }catch(e){} }
if(!sidebar||!sbList||!menuBtn||!chatArea||!msgInput||!sendBtn){ showInitErr('Missing element: sidebar='+!!sidebar+' sbList='+!!sbList+' menuBtn='+!!menuBtn+' chatArea='+!!chatArea+' msgInput='+!!msgInput+' sendBtn='+!!sendBtn); return; }
var sbActionsOverlay=document.getElementById('sbActionsOverlay');
var sbActionsOverlayBackdrop=sbActionsOverlay?sbActionsOverlay.querySelector('.sb-actions-overlay-backdrop'):null;
var sbActionsFourFloat=sbActionsOverlay?sbActionsOverlay.querySelector('.sb-actions-four-float'):null;
var _sbActionsAutoCloseTimer=null;
function closeSbActionsOverlay(){
  if(_sbActionsAutoCloseTimer){clearTimeout(_sbActionsAutoCloseTimer);_sbActionsAutoCloseTimer=null;}
  if(!sbActionsOverlay)return;
  var openId=sbActionsOverlay.dataset.openId;
  function done(){
    sbActionsOverlay.setAttribute('aria-hidden','true');sbActionsOverlay.removeAttribute('data-open-id');if(sbActionsOverlay.dataset.openId)delete sbActionsOverlay.dataset.openId;
    if(openId){var q=String(openId).replace(/\\/g,'\\\\').replace(/"/g,'\\"');var w=document.querySelector('.sb-item-actions-wrap[data-id="'+q+'"]');if(w)w.classList.remove('overlay-open');}
  }
  if(sbActionsFourFloat&&typeof poofDismiss==='function'){
    var r=sbActionsFourFloat.getBoundingClientRect();
    var phantom=document.createElement('div');phantom.style.cssText='position:fixed;left:'+r.left+'px;top:'+r.top+'px;width:'+Math.max(1,r.width)+'px;height:'+Math.max(1,r.height)+'px;pointer-events:none';
    document.body.appendChild(phantom);
    sbActionsOverlay.classList.remove('show');
    poofDismiss(phantom,function(){if(phantom.parentNode)phantom.parentNode.removeChild(phantom);done();});
  }else{sbActionsOverlay.classList.remove('show');done();}
}
function openSbActionsOverlay(trigger,convId,pinned,important){
  if(!sbActionsOverlay||!sbActionsFourFloat)return;
  closeSbActionsOverlay();
  var fourRoundUrl='/api/asset/file/four-round-point-connection-svgrepo-com.svg';
  var bombUrl='/api/asset/bomb/bomb-svgrepo-com.svg';
  var html='<button type="button" class="act pin-btn'+(pinned?' pinned':'')+'" data-id="'+esc(convId)+'" title="'+(pinned?'Unpin':'Pin')+'">'+(pinned?'&#9670;':'&#9671;')+'</button>'
  +'<button type="button" class="act star-btn'+(important?' starred':'')+'" data-id="'+esc(convId)+'" title="'+(important?'Unstar':'Star')+'">'+(important?'&#9733;':'&#9734;')+'</button>'
  +'<button type="button" class="act rename-btn" data-id="'+esc(convId)+'" title="Rename"><img src="'+bombUrl+'" alt="" class="sb-actions-rename-icon" width="18" height="18"></button>'
  +'<button type="button" class="act del-btn" data-id="'+esc(convId)+'" title="Delete">&#215;</button>';
  sbActionsFourFloat.innerHTML=html;
  var rect=trigger.getBoundingClientRect();
  var w=72,h=72;var left=rect.left+(rect.width/2)-w/2;var top=rect.top+(rect.height/2)-h/2;
  sbActionsFourFloat.style.left=Math.round(left)+'px';sbActionsFourFloat.style.top=Math.round(top)+'px';
  var wrap=trigger.closest('.sb-item-actions-wrap');
  if(wrap){wrap.classList.add('overlay-open');sbActionsOverlay.dataset.openId=convId;}
  sbActionsOverlay.classList.add('show');sbActionsOverlay.setAttribute('aria-hidden','false');
  trigger.classList.add('sparkle');setTimeout(function(){trigger.classList.remove('sparkle');},400);
  _sbActionsAutoCloseTimer=setTimeout(function(){_sbActionsAutoCloseTimer=null;closeSbActionsOverlay();},2500);
}
/* Chill "done thinking" sound — soft two-tone chime when Claudia finishes replying (Web Audio, no file) */
var _doneThinkingCtx=null;
function playDoneThinkingSound(){
  try{
    if(!window.AudioContext&&!window.webkitAudioContext)return;
    var Ctx=window.AudioContext||window.webkitAudioContext;
    if(!_doneThinkingCtx)_doneThinkingCtx=new Ctx();
    var ctx=_doneThinkingCtx;
    if(ctx.state==='suspended')ctx.resume();
    var gain=ctx.createGain();gain.gain.setValueAtTime(0.12,ctx.currentTime);gain.gain.exponentialRampToValueAtTime(0.001,ctx.currentTime+0.4);gain.connect(ctx.destination);
    function tone(freq,start,dur){var o=ctx.createOscillator();o.type='sine';o.frequency.setValueAtTime(freq,start);o.connect(gain);o.start(start);o.stop(start+dur);}
    tone(392,ctx.currentTime,0.12);tone(523.25,ctx.currentTime+0.14,0.14);
  }catch(e){}
}
/* Pin/Star/Delete/Restore (defined early so sidebar handler can call them) */
async function togglePin(id){
  var idStr=String(id);
  var convo=mobileConvos.find(function(c){return String(c.id)===idStr});if(!convo)return;
  var r=await fetch('/conversations/'+encodeURIComponent(idStr),mergeApiHeaders({method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({pinned:!convo.pinned})}));
  if(r.ok){
    var updated=await r.json();var idx=mobileConvos.findIndex(function(c){return String(c.id)===idStr});if(idx>=0)mobileConvos[idx]=Object.assign({},mobileConvos[idx],updated);
    var savedScroll=sbList.scrollTop;
    renderSidebar();
    requestAnimationFrame(function(){sbList.scrollTop=savedScroll;if(sbList.querySelector('.sb-list-inner'))updateVisibleRows();});
  }
}
async function toggleStar(id){
  var idStr=String(id);
  var convo=mobileConvos.find(function(c){return String(c.id)===(idStr);});if(!convo)return;
  var newVal=!convo.important;
  var r=await fetch('/conversations/engagement/mark_important',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({source:'mobile',id:idStr,important:newVal})}));
  if(r.ok){
    var idx=mobileConvos.findIndex(function(c){return String(c.id)===(idStr);});if(idx>=0)mobileConvos[idx]=Object.assign({},mobileConvos[idx],{important:newVal});
    if(searchResults!==null){var si=searchResults.findIndex(function(c){return String(c.id)===(idStr);});if(si>=0)searchResults[si]=Object.assign({},searchResults[si],{important:newVal});}
    var savedScroll=sbList.scrollTop;
    renderSidebar();
    requestAnimationFrame(function(){sbList.scrollTop=savedScroll;if(sbList.querySelector('.sb-list-inner'))updateVisibleRows();});
  }
}
async function deleteConvo(id){
  var convo=mobileConvos.find(function(c){return c.id===id});
  var name=convo?(convo.title||'this chat'):'this chat';
  if(!confirm('Archive "'+name.slice(0,40)+'"?\n\nIt will be saved in Archived Chats — nothing is permanently deleted.'))return;
  var r=await fetch('/conversations/'+encodeURIComponent(id),mergeApiHeaders({method:'DELETE'}));
  if(r.ok){
if(convo)archivedConvos=[Object.assign({},convo,{archived_at:new Date().toISOString()})].concat(archivedConvos);
mobileConvos=mobileConvos.filter(function(c){return c.id!==id});
if(currentId===id){currentId=mobileConvos[0]?mobileConvos[0].id:null;currentSrc='mobile'}
renderSidebar();
if(currentId)await loadConvo(currentId,'mobile');else{clearChat();showEmpty()}
  }
}
async function restoreConvo(id){
  var r=await fetch('/conversations/'+encodeURIComponent(id)+'/restore',mergeApiHeaders({method:'POST'}));
  if(r.ok){
var restored=await r.json();
archivedConvos=archivedConvos.filter(function(c){return c.id!==id});
mobileConvos=[restored].concat(mobileConvos);
showArchive=false;renderSidebar();
  }
}
/* Sidebar: one delegated click handler for list items (works with virtual + non-virtual) */
var _lastActTouch=0;
function handleSbListTap(e){
  if(e.target.closest('.mini-sparkline-wrapper')){e.stopPropagation();e.preventDefault();var it=e.target.closest('.sb-item');if(it)openActivityBreakdown(it.dataset.id,it.dataset.src||'mobile');return;}
  var wrap=e.target.closest('.sb-item-actions-wrap');
  var trigger=e.target.closest('.sb-actions-trigger');
  if(trigger&&wrap){e.stopPropagation();e.preventDefault();if(e.type==='touchend')_lastActTouch=Date.now();else if(Date.now()-_lastActTouch<400)return;var convId=wrap.dataset.id;var list=allConvos();var c=list.find(function(x){return String(x.id)===String(convId)});var pinned=!!(c&&c.pinned);var important=!!(c&&c.important);openSbActionsOverlay(trigger,convId,pinned,important);return;}
  var item=e.target.closest('.sb-item');if(item&&!e.target.closest('.act')&&!e.target.closest('.sb-item-actions-wrap')){closeSbActionsOverlay();switchConvo(item.dataset.id,item.dataset.src);closeSidebar();return;}
  var restore=e.target.closest('.restore-btn');if(restore){e.stopPropagation();e.preventDefault();if(e.type==='touchend')_lastActTouch=Date.now();else if(Date.now()-_lastActTouch<400)return;restoreConvo(restore.dataset.id);return;}
}
function handleSbActionsOverlayClick(e){
  if(!sbActionsOverlay||!sbActionsOverlay.classList.contains('show'))return;
  if(e.target.closest('.sb-actions-overlay-backdrop')){e.preventDefault();closeSbActionsOverlay();return;}
  var pin=e.target.closest('.pin-btn');if(pin){e.preventDefault();closeSbActionsOverlay();togglePin(pin.dataset.id);return;}
  var star=e.target.closest('.star-btn');if(star){e.preventDefault();closeSbActionsOverlay();toggleStar(star.dataset.id);return;}
  var rename=e.target.closest('.rename-btn');if(rename){e.preventDefault();var convId=rename.dataset.id;closeSbActionsOverlay();var q=String(convId).replace(/\\/g,'\\\\').replace(/"/g,'\\"');var item=document.querySelector('.sb-item[data-id="'+q+'"]');if(item)startEditSbTitle(item);return;}
  var del=e.target.closest('.del-btn');if(del){e.preventDefault();closeSbActionsOverlay();deleteConvo(del.dataset.id);return;}
}
if(sbActionsOverlay){sbActionsOverlay.addEventListener('click',handleSbActionsOverlayClick);}
function startEditSbTitle(item){
  var src=item.dataset.src;
  if(src!=='mobile')return;
  var ttlDiv=item.querySelector('.ttl');
  if(!ttlDiv)return;
  var currentText=(ttlDiv.textContent||'').replace(/\s*★\s*$/g,'').trim()||'Chat';
  var inp=document.createElement('input');
  inp.type='text';inp.className='sb-title-edit';inp.value=currentText;inp.setAttribute('maxlength','200');
  var importantHtml=ttlDiv.querySelector('.sb-item-star')?' <span class="sb-item-star" title="Important">★</span>':'';
  function commit(){
    var newTitle=(inp.value||'').trim().slice(0,200)||'Chat';
    var convId=item.dataset.id;
    inp.remove();
    ttlDiv.innerHTML=esc(newTitle)+importantHtml;
    fetch('/conversations/'+encodeURIComponent(convId),mergeApiHeaders({method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({title:newTitle})})).then(function(r){if(!r.ok)throw new Error();return r.json();}).then(function(updated){
      if(updated){var c=mobileConvos.find(function(x){return String(x.id)===String(convId)});if(c)c.title=updated.title||newTitle;var ac=archivedConvos.find(function(x){return String(x.id)===String(convId)});if(ac)ac.title=updated.title||newTitle;if(searchResults){var sc=searchResults.find(function(x){return String(x.id)===String(convId)});if(sc)sc.title=updated.title||newTitle;}ttlDiv.innerHTML=esc(updated.title||newTitle)+importantHtml;}
    }).catch(function(){ttlDiv.innerHTML=esc(currentText)+importantHtml;});
  }
  inp.addEventListener('blur',commit);
  inp.addEventListener('keydown',function(e){if(e.key==='Enter'){e.preventDefault();inp.blur();}if(e.key==='Escape'){inp.value=currentText;inp.blur();}});
  ttlDiv.innerHTML='';ttlDiv.appendChild(inp);inp.focus();inp.select();
}
sbList.addEventListener('click',handleSbListTap);
sbList.addEventListener('touchend',handleSbListTap,{passive:false});
/* Virtual scroll state (only used when list is long) */
var _sidebarRows=[],_sidebarOffsets=[],_sidebarTotalHeight=0,_virtualScrollRaf=null;
var SB_SECTION_HEIGHT=28,SB_ITEM_HEIGHT=56,SB_FOOTER_HEIGHT=44,VIRTUAL_THRESHOLD=50,BUFFER_ROWS=5;

/* ── Sidebar + viewport auto-detect (mobile = overlay, desktop = sidebar visible; works with Chrome DevTools device emulation) ── */
function openSidebar(){sidebar.classList.add('open');sideOverlay.classList.add('open');
  fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r){return r.ok?r.json():null;}).then(function(data){if(data&&Array.isArray(data.conversations)){mobileConvos=data.conversations;renderSidebar();}}).catch(function(){});}
function closeSidebar(){sidebar.classList.remove('open');sideOverlay.classList.remove('open')}
var _mq=window.matchMedia&&window.matchMedia('(min-width: 768px)');
function isViewportDesktop(){return _mq&&_mq.matches}
function applyViewport(){
  var desktop=isViewportDesktop();
  document.body.classList.toggle('viewport-desktop',desktop);
  if(appEl){if(desktop)closeSidebar();appEl.classList.remove('sidebar-collapsed');}
}
applyViewport();
if(_mq){_mq.addEventListener('change',applyViewport);}
if(typeof window!=='undefined'){window.addEventListener('load',function(){applyViewport();});}
menuBtn.addEventListener('click',function(){
  if(isViewportDesktop()){if(appEl)appEl.classList.toggle('sidebar-collapsed')}
  else{sidebar.classList.contains('open')?closeSidebar():openSidebar()}
});
sideOverlay.addEventListener('click',closeSidebar);
var sbUserBtn=document.getElementById('sbUserBtn'),sbUserPanel=document.getElementById('sbUserPanel');
function refreshMobileConvos(){
  fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r){return r.ok?r.json():null;}).then(function(d){if(d&&Array.isArray(d.conversations)){mobileConvos=d.conversations;renderSidebar();}}).catch(function(){});
}
if(sbUserBtn&&sbUserPanel){sbUserBtn.addEventListener('click',function(){var open=sbUserPanel.classList.toggle('open');sbUserBtn.classList.toggle('open',open);sbUserBtn.setAttribute('aria-expanded',open?'true':'false');if(open)refreshMobileConvos();});}
if(typeof document.addEventListener==='function'){document.addEventListener('visibilitychange',function(){if(document.visibilityState==='visible')refreshMobileConvos();});}
var sbRefreshBtn=document.getElementById('sbRefreshBtn');
if(sbRefreshBtn){sbRefreshBtn.addEventListener('click',function(){sbRefreshBtn.disabled=true;sbRefreshBtn.textContent='…';loadAll().then(function(){sbRefreshBtn.disabled=false;sbRefreshBtn.textContent='Refresh';}).catch(function(){sbRefreshBtn.disabled=false;sbRefreshBtn.textContent='Refresh';});});}
/* ── Header TV icon: mood from last message (thinking/laughing/judgy/silly) ── */
function moodFromText(text){
  if(!text)return 'thinking';
  var t=text.toLowerCase().trim();
  if(/\b(lol|lmao|haha|hehe|xd|😂|🤣|😹|funny|hilarious|dead)\b|\.{2,}h/.test(t))return 'laughing';
  if(/\b(why did you|really\?|seriously|smh|judgy|side.?eye|eyeroll)\b|🙄|👀/.test(t))return 'judgy';
  if(/\b(silly|dumb|goofy|weird|wtf|blah|bleh|:\?|:\/)\b|😛|😜|🤪/.test(t))return 'silly';
  return 'thinking';
}
function setHeaderMood(mood){
  if(!hdrAvatar)return;
  hdrAvatar.classList.remove('state-thinking','state-laughing','state-judgy','state-silly');
  if(mood)hdrAvatar.classList.add('state-'+mood);
}
if(hdrAvatar){
  fetch('/header_icon.svg').then(function(r){return r.text()}).then(function(svg){hdrAvatar.innerHTML=svg}).catch(function(){hdrAvatar.innerHTML='<img src="/icon.svg" alt="">'});
}
try{
/* ── Helpers ── */
function esc(s){return(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;')}
function allConvos(){
  var mobile=(searchResults!==null&&(sbSearch.value||'').trim())?searchResults:mobileConvos;
  var list=mobile.map(function(c){return Object.assign({},c,{source:c.source||'mobile'})});
  if(signedInUser&&searchResults===null){
    list=list.concat(continueConvos.map(function(c){return Object.assign({},c,{source:'continue'})}))
      .concat(grokConvos.map(function(c){return Object.assign({},c,{source:'grok'})}))
      .concat(cursorConvos.map(function(c){return Object.assign({},c,{source:'cursor'})}));
  }
  return list;
}
function srcLabel(s){return{mobile:'Mobile',continue:'VS Code',grok:'Grok',cursor:'Cursor'}[s]||s}
function srcBadge(s){return'<span class="badge '+s+'">'+srcLabel(s)+'</span>'}
function fmtDate(iso){
  if(!iso)return'';
  try{
var d=new Date(iso),now=new Date(),diff=now-d;
if(diff<60000)return'just now';
if(diff<3600000)return Math.floor(diff/60000)+'m ago';
if(diff<86400000)return Math.floor(diff/3600000)+'h ago';
if(diff<604800000)return['Sun','Mon','Tue','Wed','Thu','Fri','Sat'][d.getDay()];
return d.toLocaleDateString(undefined,{month:'short',day:'numeric'});
  }catch(e){return''}
}

/* ── Sidebar Render ── */
var _semanticSearch=false; /* toggled by ✨ button; when true, uses vector search via Chimera */
function starOrbitHtml(){
  return ' <span class="sb-item-star-wrap" title="Important"><span class="star-orbit-sparkles">'
    +'<span class="star-orbit-wrap star-orbit-inner star-orbit-tilt-in" style="--orbit-duration:2s;--orbit-phase:0"><span class="star-orbit-dot"></span></span>'
    +'<span class="star-orbit-wrap star-orbit-outer star-orbit-tilt-out star-orbit-rev" style="--orbit-duration:2.2s;--orbit-phase:0.3s"><span class="star-orbit-dot"></span></span>'
    +'</span></span>';
}
async function apiSearchConversations(query,semantic){
  try{
    var url='/api/search-conversations?q='+encodeURIComponent(query)+(semantic?'&semantic=true':'');
    var resp=await fetch(url);
    if(!resp.ok)return null;
    return await resp.json();
  }catch(_){return null;}
}
function renderSidebar(){
  var q=(sbSearch.value||'').trim();
  var qLower=q.toLowerCase();
  /* Archive view */
  if(showArchive){
var archived=archivedConvos.filter(function(c){return !q||(c.title||'').toLowerCase().indexOf(qLower)!==-1});
var html='<div style="display:flex;align-items:center;justify-content:space-between;padding:8px 12px 4px">'
  +'<span class="sb-section" style="padding:0">&#128451; Archived ('+archived.length+')</span>'
  +'<button onclick="showArchive=false;renderSidebar()" style="background:none;border:none;color:var(--pink);font-size:12px;cursor:pointer;font-weight:700">&#8592; Back</button>'
  +'</div>';
if(!archived.length)html+='<div style="padding:24px;text-align:center;color:#555;font-size:13px;">No archived chats</div>';
else html+=archived.map(function(c){
  return'<div class="sb-item" style="opacity:.75" role="button" tabindex="0" data-id="'+esc(c.id)+'" data-src="mobile">'
    +'<div class="txt"><div class="ttl">'+esc(c.title||'Chat')+'</div>'
    +'<div class="ts">archived &middot; '+(c.updated_at?fmtDate(c.updated_at):'')+'</div></div>'
    +'<button type="button" class="act restore-btn" data-id="'+esc(c.id)+'" title="Restore" style="background:#000 !important;border:none !important;color:#ff99ee !important;box-shadow:0 0 16px rgba(255,100,235,.65),0 0 32px rgba(230,60,255,.45)">&#8617; Restore</button>'
    +'</div>';
}).join('');
sbList.innerHTML=html;
sbList.querySelectorAll('.restore-btn').forEach(function(btn){ btn.addEventListener('click',function(e){ e.stopPropagation(); restoreConvo(btn.dataset.id); }); });
return;
  }
  /* If searching, use API search results; otherwise show all conversations */
  var all;
  if(q&&searchResults){
    all=searchResults.map(function(r){return{id:r.id,title:r.title,source:'mobile',updated_at:r.date,_searchSnippet:r.snippet,_searchScore:r.score};});
  }else{
    all=allConvos().filter(function(c){
if(!q)return true;
return(c.title||'').toLowerCase().indexOf(qLower)!==-1||(c.id||'').toLowerCase().indexOf(qLower)!==-1;
    });
  }
  var pinned=all.filter(function(c){return c.source==='mobile'&&c.pinned});
  var rest=all.filter(function(c){return!(c.source==='mobile'&&c.pinned)});
  var rows=[];
  if(pinned.length){rows.push({type:'section',label:'&#128204; Pinned',height:SB_SECTION_HEIGHT});pinned.forEach(function(c){rows.push({type:'item',convo:c,height:SB_ITEM_HEIGHT});});}
  if(rest.length){if(pinned.length)rows.push({type:'section',label:'Recent',height:SB_SECTION_HEIGHT});rest.forEach(function(c){rows.push({type:'item',convo:c,height:SB_ITEM_HEIGHT});});}
  if(archivedConvos.length)rows.push({type:'footer',height:SB_FOOTER_HEIGHT});
  var totalRows=rows.length;
  if(totalRows>VIRTUAL_THRESHOLD&&totalRows>0){
    var offsets=[];offsets[0]=0;for(var i=0;i<rows.length;i++)offsets[i+1]=offsets[i]+rows[i].height;
    var totalHeight=offsets[rows.length];
    _sidebarRows=rows;_sidebarOffsets=offsets;_sidebarTotalHeight=totalHeight;
    sbList.innerHTML='';
    var inner=document.createElement('div');inner.className='sb-list-inner';inner.style.cssText='height:'+totalHeight+'px;position:relative;';
    var visual=document.createElement('div');visual.className='sb-list-visual';visual.style.cssText='position:absolute;top:0;left:0;right:0;min-height:'+totalHeight+'px;';
    inner.appendChild(visual);sbList.appendChild(inner);
    if(!sbList._virtualScrollOn){sbList._virtualScrollOn=true;sbList.addEventListener('scroll',function(){if(_virtualScrollRaf)return;_virtualScrollRaf=requestAnimationFrame(function(){_virtualScrollRaf=null;updateVisibleRows();});});}
    updateVisibleRows();
    return;
  }
  var html='';
  if(pinned.length){html+='<div class="sb-section">&#128204; Pinned</div>';html+=pinned.map(renderSbItem).join('')}
  if(rest.length){if(pinned.length)html+='<div class="sb-section">Recent</div>';html+=rest.map(renderSbItem).join('')}
  if(!html){var searchTip=(q)?'<br><span style="font-size:11px;color:#888;margin-top:6px;display:inline-block">No matches — try ⭐ for starred or ✨ for semantic search</span>':'';html='<div style="padding:24px;text-align:center;color:#555;font-size:13px;">No chats yet'+searchTip+'</div>';}
  var archivedMatchCount=q?archivedConvos.filter(function(c){return(c.title||'').toLowerCase().indexOf(q)!==-1;}).length:archivedConvos.length;
  if(archivedConvos.length)html+='<button type="button" onclick="showArchive=true;renderSidebar()" role="button" style="width:100%;padding:12px;text-align:center;font-size:13px;color:var(--pink);cursor:pointer;border-top:1px solid var(--border);margin-top:4px;background:rgba(255,122,217,.08);border-left:none;border-right:none;border-bottom:none;font-weight:600">&#128451; '+(archivedMatchCount||archivedConvos.length)+' archived chat'+(archivedConvos.length!==1?'s':'')+(q&&archivedMatchCount?(archivedMatchCount===1?' match':' matches'):'')+' — Tap to open</button>';
  sbList.innerHTML=html;
}
function renderSbItem(c){
  var isActive=c.id===currentId&&c.source===currentSrc;
  var isMobile=c.source==='mobile';
  var ts=fmtDate(c.updated_at);
  var spark=(c.sparkline_data&&Array.isArray(c.sparkline_data)&&c.sparkline_data.length)?c.sparkline_data:[];
  var sparkHtml='';
  if(spark.length){
    sparkHtml='<div class="mini-sparkline-wrapper" title="Tap for activity breakdown (messages, files, code, media)" aria-label="Activity breakdown">'+spark.map(function(v){
      var pct=Math.max(8,(v/10)*100);
      return '<div class="spark-bar" style="height:'+pct+'%"></div>';
    }).join('')+'</div>';
  }
  var fourRoundUrl='/api/asset/file/four-round-point-connection-svgrepo-com.svg';
  var bombUrl='/api/asset/bomb/bomb-svgrepo-com.svg';
  return '<div class="sb-item'+(isActive?' active':'')+(isMobile&&c.pinned?' pinned':'')+(isMobile&&c.important?' important':'')+'" role="button" tabindex="0" data-id="'+esc(c.id)+'" data-src="'+c.source+'">'
+(isMobile
  ?'<div class="sb-item-actions-wrap" data-id="'+esc(c.id)+'">'
  +'<button type="button" class="sb-actions-trigger act" title="Actions" aria-label="Pin, Star, Rename, Delete"><img src="'+fourRoundUrl+'" alt="" class="sb-actions-trigger-icon" width="20" height="20"></button>'
  +'<div class="sb-actions-four">'
  +'<button type="button" class="act pin-btn'+(c.pinned?' pinned':'')+'" data-id="'+esc(c.id)+'" title="'+(c.pinned?'Unpin':'Pin')+'">'+(c.pinned?'&#9670;':'&#9671;')+'</button>'
  +'<button type="button" class="act star-btn'+(c.important?' starred':'')+'" data-id="'+esc(c.id)+'" title="'+(c.important?'Unstar':'Star')+'">'+(c.important?'&#9733;':'&#9734;')+'</button>'
  +'<button type="button" class="act rename-btn" data-id="'+esc(c.id)+'" title="Rename"><img src="'+bombUrl+'" alt="" class="sb-actions-rename-icon" width="18" height="18"></button>'
  +'<button type="button" class="act del-btn" data-id="'+esc(c.id)+'" title="Delete">&#215;</button>'
  +'</div></div>'
  :'')
+'<div class="txt"><div class="ttl">'+esc(c.title||'Chat')+(isMobile&&c.important?starOrbitHtml():'')+'</div>'
+'<div class="ts-row"><div class="ts">'+(ts?ts+' \u00B7 ':'')+srcLabel(c.source)+'</div>'+sparkHtml+'</div></div>'
+(isMobile
  ?''
  :srcBadge(c.source))
+'</div>';
}
function updateVisibleRows(){
  var inner=sbList.querySelector('.sb-list-inner');if(!inner)return;
  var visual=inner.querySelector('.sb-list-visual');if(!visual||!_sidebarRows.length)return;
  var scrollTop=sbList.scrollTop,clientHeight=sbList.clientHeight;
  var start=0,end=_sidebarRows.length-1;
  for(var i=0;i<_sidebarRows.length;i++){if(_sidebarOffsets[i+1]>scrollTop){start=Math.max(0,i-BUFFER_ROWS);break;}}
  for(var j=_sidebarRows.length-1;j>=0;j--){if(_sidebarOffsets[j]<scrollTop+clientHeight){end=Math.min(_sidebarRows.length-1,j+BUFFER_ROWS);break;}}
  visual.innerHTML='';
  for(var i=start;i<=end;i++){
    var r=_sidebarRows[i],off=_sidebarOffsets[i],h=r.height;
    var wrap=document.createElement('div');wrap.style.cssText='position:absolute;left:0;right:0;top:'+off+'px;height:'+h+'px;';
    if(r.type==='section'){wrap.className='sb-section';wrap.style.padding='8px 12px 4px';wrap.innerHTML=r.label;}
    else if(r.type==='item'){wrap.innerHTML=renderSbItem(r.convo);}
    else if(r.type==='footer'){var archCount=archivedConvos.length;wrap.innerHTML='<button type="button" onclick="showArchive=true;renderSidebar()" role="button" style="width:100%;padding:12px;text-align:center;font-size:13px;color:var(--pink);cursor:pointer;border-top:1px solid var(--border);margin-top:4px;background:rgba(255,122,217,.08);border-left:none;border-right:none;border-bottom:none;font-weight:600">&#128451; '+archCount+' archived chat'+(archCount!==1?'s':'')+' — Tap to open</button>';}
    visual.appendChild(wrap);
  }
}
function toggleSemanticSearch(btn){
  _semanticSearch=!_semanticSearch;
  btn.style.color=_semanticSearch?'var(--pink)':'#888';
  btn.style.borderColor=_semanticSearch?'var(--pink)':'var(--border)';
  btn.title=_semanticSearch?'Semantic search ON (vector via Chimera) — click to use keyword':'Semantic search (vector via Chimera)';
  /* Re-run search with new mode if there's a query */
  var q=(sbSearch.value||'').trim();
  if(q){
    apiSearchConversations(q,_semanticSearch).then(function(results){
      searchResults=(results&&results.length)?results:null;
      renderSidebar();
    }).catch(function(){searchResults=null;renderSidebar();});
  }
}
var _sbSearchTimer=null;
sbSearch.addEventListener('input',function(){
  var self=this;
  clearTimeout(_sbSearchTimer);
  _sbSearchTimer=setTimeout(function(){
    var q=(self.value||'').trim();
    if(q){
      apiSearchConversations(q,_semanticSearch).then(function(results){
        searchResults=(results&&results.length)?results:null;
        renderSidebar();
      }).catch(function(){
        searchResults=null;
        renderSidebar();
      });
    }else{searchResults=null;renderSidebar();}
  },180);
});

/* ── Activity Breakdown (tap sparkline → overlay; click bucket → bigger view) ── */
var activityOverlay=document.getElementById('activityBreakdownOverlay');
var activityTitleEl=activityOverlay?activityOverlay.querySelector('.activity-breakdown-title'):null;
var activityBucketsEl=activityOverlay?activityOverlay.querySelector('.activity-buckets'):null;
var BUCKETS=[{key:'message_count',label:'Messages',color:'rgba(80,180,255,.9)',icon:'\uD83D\uDCAC'},{key:'files',label:'Files',color:'rgba(80,220,120,.9)',icon:'\uD83D\uDCC4'},{key:'code_snippets',label:'Code Snippets',color:'rgba(255,180,80,.9)',icon:'\uD83D\uDCBB'},{key:'media',label:'Media',color:'rgba(255,100,200,.9)',icon:'\uD83D\uDDBC'},{key:'feedback_liked',label:'Liked',color:'rgba(180,220,100,.9)',icon:'\uD83D\uDC4D'},{key:'feedback_noted',label:'Noted',color:'rgba(255,120,80,.9)',icon:'\uD83D\uDC4E'}];
var ICON_BUBBLE_NEON='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="14" height="14" class="icon-bubble-neon" aria-hidden="true"><style>.st0{fill:currentColor}</style><g><path class="st0" d="M133.048,121.218c-1.663,0-3.296,0.337-4.841,0.996c-20.036,8.606-36.119,24.218-45.306,43.973c-1.381,2.966-1.522,6.3-0.4,9.375c1.122,3.083,3.382,5.546,6.371,6.936c1.64,0.761,3.373,1.146,5.17,1.146c4.762,0,9.14-2.794,11.14-7.116c6.646-14.27,18.256-25.544,32.715-31.726c3.013-1.294,5.342-3.68,6.567-6.732c1.216-3.044,1.177-6.386-0.118-9.398C142.408,124.144,137.967,121.218,133.048,121.218z"/><path class="st0" d="M325.854,203.342c-0.016-89.821-73.11-162.915-162.932-162.931C73.102,40.427,0.015,113.521,0,203.342c0.015,89.821,73.102,162.908,162.923,162.924C252.744,366.25,325.838,293.163,325.854,203.342z M162.923,334.344c-34.974-0.008-67.869-13.636-92.629-38.372c-24.736-24.768-38.364-57.664-38.372-92.63c0.008-34.982,13.635-67.877,38.372-92.63c24.775-24.743,57.671-38.371,92.629-38.379c34.967,0.008,67.862,13.636,92.63,38.379c24.744,24.768,38.372,57.664,38.38,92.63c-0.008,34.959-13.635,67.854-38.38,92.63C230.793,320.708,197.898,334.336,162.923,334.344z"/><path class="st0" d="M427.458,69.815c-46.6,0.008-84.532,37.932-84.549,84.541c0.016,46.601,37.948,84.525,84.549,84.541c46.601-0.016,84.526-37.94,84.542-84.541C511.984,107.747,474.06,69.823,427.458,69.815z M464.661,191.575c-9.963,9.924-23.175,15.392-37.203,15.4c-14.035-0.008-27.247-5.476-37.218-15.408c-9.924-9.963-15.392-23.175-15.4-37.21c0.008-14.035,5.476-27.246,15.408-37.219c9.963-9.924,23.175-15.392,37.21-15.4c14.028,0.008,27.24,5.477,37.211,15.408c9.924,9.964,15.4,23.184,15.408,37.211C480.07,168.383,474.593,181.603,464.661,191.575z"/><path class="st0" d="M349.076,251.325c-2.683,10.434-6.261,20.664-10.654,30.487c16.146,2.808,30.761,10.379,42.428,22.03c15.024,15.047,23.292,35.029,23.301,56.258c-0.008,21.23-8.277,41.212-23.301,56.258c-15.048,15.024-35.03,23.301-56.258,23.309c-21.23-0.008-41.212-8.284-56.266-23.309c-11.015-11.03-18.421-24.822-21.559-40.042c-9.666,4.691-19.746,8.574-30.056,11.572c12.655,49.386,56.699,83.686,107.882,83.701c61.46-0.015,111.473-50.029,111.489-111.489C436.065,308.038,399.608,262.661,349.076,251.325z"/></g></svg>';
var ICON_BUBBLES_POP_NEON='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="20" height="20" class="icon-bubbles-pop-neon" aria-hidden="true"><style>.st0{fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round;stroke-miterlimit:10}</style><circle class="st0" cx="22" cy="22" r="7"/><path class="st0" d="M22,19c1,0,1.9,0.5,2.5,1.3"/><circle class="st0" cx="9" cy="9" r="4"/><circle class="st0" cx="5.5" cy="21.5" r="3.5"/><line class="st0" x1="19" y1="3" x2="19" y2="6"/><line class="st0" x1="26" y1="10" x2="23" y2="10"/><line class="st0" x1="23.9" y1="5.1" x2="21.8" y2="7.2"/></svg>';
var ICON_COPY_NEON='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="14" height="14" class="icon-copy-neon" aria-hidden="true"><rect x="9" y="9" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" rx="1"/><rect x="2" y="2" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" rx="1"/></svg>';
var ICON_FORK='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 2v8"/><path d="M6 22V10l6-6"/><path d="M18 22V10l-6-6"/></svg>';
function openImageLightbox(src){
  if(!src)return;
  var overlay=document.createElement('div');
  overlay.className='chat-img-lightbox';overlay.setAttribute('role','dialog');overlay.setAttribute('aria-modal','true');overlay.setAttribute('aria-label','Photo full size');
  overlay.style.cssText='position:fixed;inset:0;z-index:100000;background:rgba(0,0,0,.92);display:flex;align-items:center;justify-content:center;padding:16px;box-sizing:border-box;';
  var img=document.createElement('img');img.src=src;img.alt='';img.style.cssText='max-width:100%;max-height:100%;object-fit:contain;border-radius:8px;';
  overlay.appendChild(img);
  function close(){overlay.remove();document.body.style.overflow='';}
  overlay.addEventListener('click',close);img.addEventListener('click',function(e){e.stopPropagation();});
  document.body.style.overflow='hidden';document.body.appendChild(overlay);
}
function getBubblePlainText(bubble){
  if(!bubble)return'';
  var t=bubble.innerText||bubble.textContent||'';
  return (t.replace(/\r\n/g,'\n').replace(/\r/g,'\n')||'').trim();
}
function copyMessageAndFeedback(btn,bubble){
  var wrap=bubble?bubble.closest('.msg-wrap'):null;
  var el=wrap?wrap.querySelector('.bubble'):bubble;
  var text=getBubblePlainText(el);
  if(!text)return;
  var label=btn.getAttribute('aria-label')||'Copy';
  var origHtml=btn.innerHTML;
  function done(ok){btn.innerHTML=ok?'<span class="copy-feedback">Copied!</span>':origHtml;btn.setAttribute('aria-label',label);if(ok)setTimeout(function(){btn.innerHTML=origHtml;},1600);}
  function tryClipboard(){
    if(navigator.clipboard&&navigator.clipboard.writeText)return navigator.clipboard.writeText(text).then(function(){done(true);}).catch(function(){tryFallback();});
    tryFallback();
  }
  function tryFallback(){
    var ta=document.createElement('textarea');ta.value=text;ta.setAttribute('readonly','');ta.style.cssText='position:fixed;left:-9999px;top:0';document.body.appendChild(ta);ta.select();ta.setSelectionRange(0,text.length);
    var ok=false;try{ok=document.execCommand('copy');}catch(e){}document.body.removeChild(ta);done(ok);
  }
  tryClipboard();
}
function copyConversationToClipboard(){
  if(!currentMessages||!currentMessages.length)return;
  var lines=[];
  currentMessages.forEach(function(m){
    var role=(m.role||'').toLowerCase();
    var content=(m.content||'').trim();
    var label=role==='user'?getCurrentUserDisplayName():'Locus';
    lines.push(label+': '+content);
  });
  var text=lines.join('\n\n');
  if(!text)return;
  function done(ok){
    if(hdrCopyExportBtn){hdrCopyExportBtn.dataset.copied=ok?'1':'';hdrCopyExportBtn.setAttribute('aria-label',ok?'Copied':(hdrCopyExportBtn.dataset.originalLabel||'Copy or export conversation'));if(ok)setTimeout(function(){hdrCopyExportBtn.dataset.copied='';},1600);}
  }
  function tryFallback(){
    var ta=document.createElement('textarea');ta.value=text;ta.setAttribute('readonly','');ta.style.cssText='position:fixed;left:-9999px;top:0';document.body.appendChild(ta);ta.select();ta.setSelectionRange(0,text.length);
    var ok=false;try{ok=document.execCommand('copy');}catch(e){}document.body.removeChild(ta);done(ok);
  }
  if(navigator.clipboard&&navigator.clipboard.writeText)navigator.clipboard.writeText(text).then(function(){done(true);}).catch(tryFallback);
  else tryFallback();
}
function exportConversationAsMarkdown(){
  if(!currentMessages||!currentMessages.length)return;
  var lines=[];
  currentMessages.forEach(function(m){
    var role=(m.role||'').toLowerCase();
    var content=(m.content||'').trim();
    var label=role==='user'?getCurrentUserDisplayName():'Locus';
    lines.push('## '+label+'\n\n'+content+'\n');
  });
  var md=lines.join('\n');
  var now=new Date();
  var y=now.getFullYear(),mo=String(now.getMonth()+1).padStart(2,'0'),d=String(now.getDate()).padStart(2,'0');
  var h=String(now.getHours()).padStart(2,'0'),mi=String(now.getMinutes()).padStart(2,'0');
  var name='conversation-'+y+'-'+mo+'-'+d+'-'+h+mi+'.md';
  var blob=new Blob([md],{type:'text/markdown;charset=utf-8'});
  var a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download=name;a.style.display='none';document.body.appendChild(a);a.click();document.body.removeChild(a);URL.revokeObjectURL(a.href);
}
function updateCopyConvoButtonVisibility(){
  var show=currentMessages&&currentMessages.length>0;
  if(hdrCopyExportWrap){hdrCopyExportWrap.style.display=show?'inline-flex':'none';if(show&&hdrCopyExportBtn&&!hdrCopyExportBtn.dataset.originalLabel)hdrCopyExportBtn.dataset.originalLabel=hdrCopyExportBtn.getAttribute('aria-label')||'Copy or export';}
  if(forkConvoBtn)forkConvoBtn.style.display=show?'inline-flex':'none';
}
function closeCopyExportDropdown(){
  if(hdrCopyExportDropdown){hdrCopyExportDropdown.classList.remove('open');hdrCopyExportDropdown.setAttribute('aria-hidden','true');}
  if(hdrCopyExportBtn){hdrCopyExportBtn.setAttribute('aria-expanded','false');}
}
function sparkleBurst(container){
  var wrap=document.createElement('div');wrap.className='pop-sparkles';wrap.setAttribute('aria-hidden','true');
  var positions=[[50,0],[85,15],[100,50],[85,85],[50,100],[15,85],[0,50],[15,15]];
  var deltas=[[0,-10],[7,-7],[10,0],[7,7],[0,10],[-7,7],[-10,0],[-7,-7]];
  for(var i=0;i<8;i++){var s=document.createElement('span');s.className='pop-sparkle-dot';s.style.left=positions[i][0]+'%';s.style.top=positions[i][1]+'%';s.style.animationDelay=(i*0.04)+'s';s.style.setProperty('--sparkle-dx',deltas[i][0]+'px');s.style.setProperty('--sparkle-dy',deltas[i][1]+'px');wrap.appendChild(s);}
  container.appendChild(wrap);
  setTimeout(function(){if(wrap.parentNode)wrap.parentNode.removeChild(wrap);},600);
}
function poofDismiss(element,callback){
  if(!element||!element.getBoundingClientRect){if(callback)callback();return;}
  var r=element.getBoundingClientRect();
  var pad=28;var w=Math.max(80,r.width+pad*2);var h=Math.max(80,r.height+pad*2);var x=r.left+r.width/2-w/2;var y=r.top+r.height/2-h/2;
  element.classList.add('poof-dismissing');
  var overlay=document.createElement('div');overlay.className='poof-overlay';overlay.setAttribute('aria-hidden','true');
  overlay.style.left=x+'px';overlay.style.top=y+'px';overlay.style.width=w+'px';overlay.style.height=h+'px';
  var cloud=document.createElement('div');cloud.className='poof-cloud';cloud.style.left='50%';cloud.style.top='50%';cloud.style.width=w+'px';cloud.style.height=h+'px';cloud.style.marginLeft=-w/2+'px';cloud.style.marginTop=-h/2+'px';overlay.appendChild(cloud);
  var colors=['#ff7ad9','#ff99ee','#ffcc66','#ffe066','#ffb6e6'];
  var angles=[0,45,90,135,180,225,270,315,22,67,112,157,202,247,292,337];
  for(var i=0;i<24;i++){var a=(angles[i%16]+(i*7))*(Math.PI/180);var dist=25+Math.random()*35;var dx=Math.cos(a)*dist;var dy=Math.sin(a)*dist;var s=document.createElement('span');s.className='poof-sparkle';s.style.setProperty('--poof-dx',dx+'px');s.style.setProperty('--poof-dy',dy+'px');s.style.animationDelay=(i*0.02)+'s';s.style.color=colors[i%colors.length];overlay.appendChild(s);}
  document.body.appendChild(overlay);
  setTimeout(function(){if(overlay.parentNode)overlay.parentNode.removeChild(overlay);if(callback)callback();},620);
}
var TREE_NODES=7;
var SPARK_COUNT=12;
var ACTIVITY_ORNAMENT_POSITIONS=[{left:10,top:18},{left:28,top:6},{left:48,top:24},{left:68,top:8},{left:82,top:20},{left:18,top:32},{left:55,top:4}];
var ACTIVITY_BRANCHES=['branch-svgrepo-com.svg','leaves-branch-svgrepo-com.svg','naked-trees-branches-svgrepo-com.svg','tree-in-winter-tree-branch-winter-svgrepo-com.svg','tree11-svgrepo-com.svg'];
var ACTIVITY_PETALS=['flower-blossom-svgrepo-com.svg','flower-svgrepo-com.svg','flower-with-petals-svgrepo-com.svg','flower-rose-svgrepo-com.svg','flower-rose-svgrepo-com (1).svg','flower-rose-svgrepo-com (2).svg','flower-smile-svgrepo-com.svg','flower-svgrepo-com (1).svg','flower-svgrepo-com (2).svg','flower-svgrepo-com (3).svg','flower-svgrepo-com (4).svg','flower-svgrepo-com (5).svg','flower-svgrepo-com (6).svg','flower-svgrepo-com (7).svg','flower-svgrepo-com (8).svg','flower-svgrepo-com (9).svg','flower-svgrepo-com (10).svg','flower-svgrepo-com (11).svg','flower-svgrepo-com (12).svg','shamrock-clover-svgrepo-com.svg'];
var ACTIVITY_ASSET_BASE='/api/asset/bucket_tree_flowers/';
function activityTreeHtml(color,pct,count){
  var lit=count>0?Math.min(TREE_NODES,count):0;
  var full=lit>=TREE_NODES;
  var branchFile=ACTIVITY_BRANCHES[Math.floor(Math.random()*ACTIVITY_BRANCHES.length)];
  var branchUrl=ACTIVITY_ASSET_BASE+encodeURIComponent('bucket tree/'+branchFile);
  var html='<div class="activity-tree-wrap activity-branch-wrap'+(full?' tree-full':'')+'" style="--tree-color:'+color+'" aria-hidden="true">';
  html+='<div class="activity-branch-bg" style="-webkit-mask-image:url('+branchUrl+');mask-image:url('+branchUrl+');background:var(--tree-color)"></div>';
  html+='<div class="activity-tree activity-branch-nodes">';
  for(var i=0;i<TREE_NODES;i++){
    var pos=ACTIVITY_ORNAMENT_POSITIONS[i]||{left:50,top:50};
    var isLit=i<lit;
    var petalFile=ACTIVITY_PETALS[Math.floor(Math.random()*ACTIVITY_PETALS.length)];
    var petalUrl=ACTIVITY_ASSET_BASE+encodeURIComponent('pedals/'+petalFile);
    html+='<div class="activity-tree-node activity-branch-node'+(isLit?' lit':'')+'" style="--node-color:'+color+';left:'+pos.left+'%;top:'+pos.top+'%" aria-hidden="true"><img class="activity-node-flower" src="'+petalUrl+'" alt="" role="presentation"></div>';
  }
  html+='</div>';
  if(full){
    html+='<div class="activity-tree-sparks" aria-hidden="true">';
    for(var s=0;s<SPARK_COUNT;s++)html+='<span class="tree-spark" style="animation-delay:'+(s*0.08)+'s;left:'+(15+Math.random()*70)+'%"></span>';
    html+='</div>';
  }
  html+='</div>';
  return html;
}
var _activityBreakdownPreviousFocus=null;
function openActivityBreakdown(convId,source){
  if(!activityOverlay||!activityTitleEl||!activityBucketsEl)return;
  _activityBreakdownPreviousFocus=document.activeElement;
  var src=(source||'mobile').toLowerCase();
  var url=src==='mobile'?'/conversations/'+encodeURIComponent(convId)+'/activity':'/activity?source='+encodeURIComponent(src)+'&id='+encodeURIComponent(convId);
  fetch(url).then(function(r){return r.json();}).then(function(data){
    activityTitleEl.textContent=(data.title||'Chat')+' — Activity Breakdown';
    var maxVal=Math.max(1,data.message_count||0,data.files||0,data.code_snippets||0,data.media||0,data.feedback_liked||0,data.feedback_noted||0);
    activityBucketsEl.innerHTML='';
    var hist=data.title_history;
    if(hist&&hist.length>0){
      var orig=hist[0].from;
      var withDates=hist.map(function(h){var d=h.at?new Date(h.at):null;var ds=d?d.toLocaleDateString(undefined,{month:'short',day:'numeric'})+' '+d.toLocaleTimeString(undefined,{hour:'2-digit',minute:'2-digit'}):'';return esc(h.to||'')+(ds?' <span class="activity-title-history-date">('+esc(ds)+')</span>':'');});
      var row=document.createElement('div');row.className='activity-title-history';
      row.innerHTML='<span class="activity-title-history-label">Title history</span><div class="activity-title-history-list">Originally: <strong>'+esc(orig)+'</strong></div><div class="activity-title-history-list">&#8594; '+withDates.join(' &#8594; ')+'</div>';
      activityBucketsEl.appendChild(row);
    }
    BUCKETS.forEach(function(b){
      var count=b.key==='message_count'?(data.message_count||0):(data[b.key]||0);
      var pct=maxVal?Math.max(4,(count/maxVal)*100):0;
      var row=document.createElement('div');row.className='activity-bucket';row.setAttribute('role','listitem');
      row.innerHTML='<span class="activity-bucket-label">'+b.label+'</span><span class="activity-bucket-count">'+count.toLocaleString()+'</span>'+activityTreeHtml(b.color,pct,count)+'<div class="activity-bucket-detail">'+count.toLocaleString()+' '+b.label.toLowerCase()+' in this chat.</div>';
      row.addEventListener('click',function(e){e.stopPropagation();e.currentTarget.classList.toggle('expanded');});
      activityBucketsEl.appendChild(row);
    });
    activityOverlay.classList.add('show');
    activityOverlay.setAttribute('aria-hidden','false');
    activityOverlay.removeAttribute('inert');
    var closeBtn=activityOverlay.querySelector('.activity-breakdown-close');if(closeBtn)closeBtn.focus();
  }).catch(function(){});
}
function closeActivityBreakdown(){
  if(!activityOverlay)return;
  if(_activityBreakdownPreviousFocus&&typeof _activityBreakdownPreviousFocus.focus==='function'&&document.contains(_activityBreakdownPreviousFocus)){
    _activityBreakdownPreviousFocus.focus();
  }else if(sbList){sbList.focus();}
  _activityBreakdownPreviousFocus=null;
  activityOverlay.classList.remove('show');
  activityOverlay.setAttribute('aria-hidden','true');
  activityOverlay.setAttribute('inert','');
}
if(activityOverlay){
  var backdrop=activityOverlay.querySelector('.activity-breakdown-backdrop');if(backdrop)backdrop.addEventListener('click',closeActivityBreakdown);
  var closeBtn=activityOverlay.querySelector('.activity-breakdown-close');if(closeBtn)closeBtn.addEventListener('click',closeActivityBreakdown);
}

/* ── Chat Render ── */
function clearChat(){
  var kids=Array.from(chatArea.children);
  kids.forEach(function(k){if(k!==typing&&k!==generatingImage)chatArea.removeChild(k);});
  if(typing)typing.classList.remove('show');if(generatingImage)generatingImage.classList.remove('show');
}
function showEmpty(isGroup){
  var el=document.createElement('div');el.className='empty-state';
  if(isGroup){
    el.innerHTML='<div class="em-icon">&#128101;</div><div class="em-title">Group chat</div>'
    +'<div class="em-sub">No messages yet. Say hi!</div>';
  }else{
    el.innerHTML='<div class="em-icon">&#128172;</div><div class="em-title">Start a conversation</div>'
    +'<div class="em-sub">Say anything &#8212; access your memories and search the web.</div>'
    +'<div class="em-sub" style="margin-top:8px;font-size:11px;opacity:.85">Same workspace as your PC. Open the menu to see Mobile, VS Code, Cursor &amp; Grok chats.</div>';
  }
  chatArea.insertBefore(el,typing);
}
function isImageRequest(txt){var t=(txt||'').trim().toLowerCase();return!/^\s*$/.test(t)&&(/draw\s|generate\s*(an?)?\s*(image|picture|photo|pic)|picture\s+of|create\s*(an?)?\s*image|make\s*(me\s*)?(an?)?\s*image|generate\s*me\s*(an?)?\s*(image|picture)|can you draw|draw me/i.test(t));}
function escapeHtml(s){return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');}
var CHAT_FILE_ICON_SVG='data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#8b5a8f" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/></svg>');
var EDIT_MSG_PENCIL_URL='/api/asset/pencil/pencil-svgrepo-com%20(4).svg';
var EDIT_MSG_HORNS_URL='/api/asset/horns/sign-of-the-horns-svgrepo-com.svg';
var CHAT_IMAGE_ICON_SVG='data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#8b5a8f" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/></svg>');
function appendChatFileIcons(container,files){
  if(!files||!files.length)return;
  var row=document.createElement('div');row.className='chat-attach-row';
  files.forEach(function(f){var name=typeof f==='string'?f:(f&&f.name)?f.name:'File';var ext=(name.split('.').pop()||'').toLowerCase();var isPdf=ext==='pdf';var icon=document.createElement('img');icon.className='chat-attach-icon';icon.src=CHAT_FILE_ICON_SVG;icon.alt='';icon.setAttribute('aria-hidden','true');var chip=document.createElement('span');chip.className='chat-attach-chip';chip.appendChild(icon);chip.appendChild(document.createTextNode(name));row.appendChild(chip);});
  container.appendChild(row);
}
var _draftEditorOverlay=null;
function openDraftEditor(path,title){
  if(!path)return;
  if(!_draftEditorOverlay){
    var overlay=document.createElement('div');overlay.className='draft-editor-overlay';overlay.setAttribute('aria-hidden','true');
    var modal=document.createElement('div');modal.className='draft-editor-modal';
    modal.innerHTML='<h3>Edit document</h3><textarea id="draftEditorText" placeholder="Loading\u2026"></textarea><div class="modal-actions"><button type="button" id="draftEditorCancel" class="chat-doc-edit-btn">Cancel</button><button type="button" id="draftEditorSave" class="chat-doc-files-btn">Save</button></div>';
    overlay.appendChild(modal);
    overlay.addEventListener('click',function(e){if(e.target===overlay)closeDraftEditor();});
    document.getElementById('draftEditorCancel').addEventListener('click',closeDraftEditor);
    document.body.appendChild(overlay);
    _draftEditorOverlay=overlay;
  }
  var overlay=_draftEditorOverlay;
  var modal=overlay.querySelector('.draft-editor-modal');
  var h3=modal.querySelector('h3');if(h3)h3.textContent=title||'Edit document';
  var ta=modal.querySelector('#draftEditorText');if(!ta)ta=modal.querySelector('textarea');
  ta.value='Loading\u2026';
  overlay.dataset.draftPath=path;
  overlay.classList.add('show');
  overlay.setAttribute('aria-hidden','false');
  fetch('/api/files/read?path='+encodeURIComponent(path),mergeApiHeaders({})).then(function(r){return r.ok?r.json():null;}).then(function(d){if(d&&d.content!=null)ta.value=d.content;else ta.value='';}).catch(function(){ta.value='';});
  var saveBtn=modal.querySelector('#draftEditorSave');
  saveBtn.onclick=function(){
    var p=overlay.dataset.draftPath;if(!p)return;
    saveBtn.disabled=true;
    fetch('/api/files/write',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path:p,content:ta.value})})).then(function(r){if(r.ok){closeDraftEditor();}else{saveBtn.disabled=false;}}).catch(function(){saveBtn.disabled=false;});
  };
}
function closeDraftEditor(){
  if(_draftEditorOverlay){_draftEditorOverlay.classList.remove('show');_draftEditorOverlay.setAttribute('aria-hidden','true');}
}
function parsePlanBlock(content){
  if(!content||typeof content!=='string')return null;
  var re=/(?:^|\n)(\*\*Plan\*\*|##\s*Plan|Steps:)\s*([\s\S]*?)(?=\n\n|\n##\s|\n\*\*\S|$)/im;
  var m=content.match(re);
  if(!m)return null;
  var planStart=m.index+(m[0].charAt(0)==='\n'?1:0);
  var planEnd=m.index+m[0].length;
  return{before:content.slice(0,planStart).trim(),plan:content.slice(planStart,planEnd).trim(),after:content.slice(planEnd).replace(/^\s+/,'')};
}
var DEFAULT_FOLLOWUP_SUGGESTIONS=['Tell me more','Explain that simply','Give me an example','What else?'];
var CASUAL_FOLLOWUP_SUGGESTIONS=['hehe how are you???? <3','uuuuugh hi bestie','omg the strangest thing happened....','<3'];
function isCasualMessage(text){if(!text||typeof text!=='string')return false;var t=String(text).replace(/\[THINKING:[^\]]*\]/gi,'').trim();if(t.length>55)return false;var lower=t.toLowerCase();if(/^(hey~?|hi!?|hello|heya|hiya|yo|sup|hii+|hey!)\s*$/.test(lower)||(t.length<=25&&!/[?.!;]/.test(t)))return true;return false;}
function addBubble(role,content,scroll,quickReplies,isQuickReplyChoice,opts){
  var empty=chatArea.querySelector('.empty-state');if(empty)empty.remove();
  opts=opts||{};
  if(role==='assistant'&&(currentSrc==='mobile'&&!currentRO)&&opts.style!=='thinking'&&(!quickReplies||!Array.isArray(quickReplies)||quickReplies.length===0))quickReplies=(isCasualMessage(content)?CASUAL_FOLLOWUP_SUGGESTIONS:DEFAULT_FOLLOWUP_SUGGESTIONS).slice();
  var wrap=document.createElement('div');wrap.className='msg-wrap '+(role==='user'?'user':'assistant')+(isQuickReplyChoice?' quick-reply-choice':'')+(opts.style==='thinking'?' thinking-msg':'');
  var row=document.createElement('div');row.className='msg-row';
  var av=document.createElement('div');
  if(role==='user'){av.className='msg-av user-av';var ui=document.createElement('img');ui.src=currentUserAvatarUrl;ui.alt='';ui.style.cssText='width:64px;height:64px;max-width:64px;max-height:64px;object-fit:contain;display:block;flex-shrink:0';ui.onerror=function(){av.textContent='\u273F';};av.appendChild(ui);}
  else{av.className='msg-av';var ai=document.createElement('img');ai.src='/locus_avatar.svg';ai.alt='';ai.style.cssText='width:40px;height:40px;max-width:40px;max-height:40px;object-fit:contain;display:block;flex-shrink:0';ai.onerror=function(){this.src='/chat_icon.png';this.onerror=function(){this.src='/icon.svg';};};av.appendChild(ai);}
  var bubble=document.createElement('div');bubble.className='bubble';
  var imgSrc=opts.imageDataUrl||(opts.imagePath?('/api/chat_image?path='+encodeURIComponent(opts.imagePath)):null);
  if(role==='user'){
    if(imgSrc){
      var imgWrap=document.createElement('div');imgWrap.className='chat-img-wrap';
      var img=document.createElement('img');img.className='chat-img';img.src=imgSrc;img.alt='Sent photo';img.loading='lazy';
      img.onclick=function(){openImageLightbox(imgSrc);};
      img.onerror=function(){imgWrap.style.display='none';};
      imgWrap.appendChild(img);bubble.appendChild(imgWrap);
    }
    if(opts.files&&opts.files.length)appendChatFileIcons(bubble,opts.files);
    bubble.appendChild(document.createTextNode((isQuickReplyChoice?'\u21B3 Chose: ':'')+(content||'')));
  }else{
    if(imgSrc){ var imgWrap=document.createElement('div');imgWrap.className='chat-img-wrap'; var img=document.createElement('img');img.className='chat-img';img.src=imgSrc;img.alt='Generated image';img.loading='lazy'; img.onclick=function(){openImageLightbox(imgSrc);}; img.onerror=function(){imgWrap.style.display='none';}; imgWrap.appendChild(img);bubble.appendChild(imgWrap); }
    if(opts.style==='thinking'){
      var details=document.createElement('details');details.className='thinking-details';
      var summary=document.createElement('summary');summary.className='thinking-summary';summary.textContent='Thought';
      var readout=document.createElement('div');readout.className='thinking-readout';readout.innerHTML=(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');
      details.appendChild(summary);details.appendChild(readout);bubble.appendChild(details);
    }else{
      var textEl=document.createElement('div');textEl.className='bubble-text';
      var parsed=parsePlanBlock(content||'');
      var html='';
      if(parsed&&parsed.plan){
        try{
          if(parsed.before)html+=(window.marked?marked.parse(parsed.before,{breaks:true,gfm:true}):escapeHtml(parsed.before).replace(/\n/g,'<br>'))+'<br>';
          html+='<details class="assistant-plan"><summary>Plan / steps</summary><div class="bubble-text">'+(window.marked?marked.parse(parsed.plan,{breaks:true,gfm:true}):escapeHtml(parsed.plan).replace(/\n/g,'<br>'))+'</div></details>';
          if(parsed.after)html+='<br>'+(window.marked?marked.parse(parsed.after,{breaks:true,gfm:true}):escapeHtml(parsed.after).replace(/\n/g,'<br>'));
        }catch(e){html=(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');}
      }else{
        try{html=window.marked?marked.parse(content||'',{breaks:true,gfm:true}):(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');}catch(e){textEl.textContent=content||'';}
      }
      if(html)textEl.innerHTML=html;
      bubble.appendChild(textEl);
    }
  }
  row.appendChild(av);row.appendChild(bubble);wrap.appendChild(row);
  if(role==='assistant'&&quickReplies&&Array.isArray(quickReplies)&&quickReplies.length>0){
    var qrRow=document.createElement('div');qrRow.className='msg-row quick-replies';
    var qrSpacer=document.createElement('div');qrSpacer.className='msg-av';qrSpacer.setAttribute('aria-hidden','true');
    var qrWrap=document.createElement('div');qrWrap.className='quick-reply-wrap';
    quickReplies.forEach(function(label){
      var btn=document.createElement('button');btn.type='button';btn.className='quick-reply-btn';btn.textContent=label;btn.dataset.option=label;
      btn.addEventListener('click',function(){sendQuickReply(label,btn);});
      qrWrap.appendChild(btn);
    });
    qrRow.appendChild(qrSpacer);qrRow.appendChild(qrWrap);wrap.appendChild(qrRow);
  }
  if(role==='assistant'&&opts.draft_documents&&Array.isArray(opts.draft_documents)&&opts.draft_documents.length>0){
    var docRow=document.createElement('div');docRow.className='msg-row chat-doc-row';
    var docSpacer=document.createElement('div');docSpacer.className='msg-av';docSpacer.setAttribute('aria-hidden','true');
    var docWrap=document.createElement('div');docWrap.className='chat-doc-cards';
    opts.draft_documents.forEach(function(doc){
      var card=document.createElement('div');card.className='chat-doc-card';
      var title=(doc.title||'Draft').trim();
      var typ=(doc.type||'draft').toLowerCase();
      var preview=(doc.content||'').trim().split('\n').slice(0,4).join('\n');
      if(preview.length>200)preview=preview.slice(0,200)+'\u2026';
      card.innerHTML='<h4>'+escapeHtml(title)+' <span class="chat-doc-type">'+escapeHtml(typ)+'</span></h4><div class="chat-doc-preview">'+escapeHtml(preview||'No preview')+'</div><div class="chat-doc-actions"></div>';
      var act=card.querySelector('.chat-doc-actions');
      var editBtn=document.createElement('button');editBtn.type='button';editBtn.className='chat-doc-edit-btn';editBtn.textContent='Edit';
      editBtn.dataset.path=doc.path;editBtn.dataset.title=title;
      editBtn.addEventListener('click',function(){openDraftEditor(editBtn.dataset.path,editBtn.dataset.title);});
      var filesBtn=document.createElement('button');filesBtn.type='button';filesBtn.className='chat-doc-files-btn';filesBtn.textContent='Open in Files';
      filesBtn.addEventListener('click',function(){window.location.href='/files?path='+encodeURIComponent(doc.path);});
      act.appendChild(editBtn);act.appendChild(filesBtn);
      docWrap.appendChild(card);
    });
    docRow.appendChild(docSpacer);docRow.appendChild(docWrap);wrap.appendChild(docRow);
  }
  chatArea.insertBefore(wrap,typing);
  if(scroll!==false)scrollBottom();
}
async function sendQuickReply(option,btnEl){
  if(currentRO||currentSrc!=='mobile'||!currentId)return;
  if(!isCurrentConvoInMobileList()){addBubble('assistant','This chat is no longer in your list. Open another chat from the sidebar.',true);return;}
  if(sendInProgress){pendingDoubleTextQueue.push({content:option,sentId:currentId});addBubble('user',option,true,null,true);if(btnEl){btnEl.disabled=true;btnEl.classList.add('used');}return;}
  var sentId=currentId;
  addBubble('user',option,true,null,true);
  if(btnEl){btnEl.disabled=true;btnEl.classList.add('used');}
  sendInProgress=true;sendBtn.disabled=true;typing.classList.add('show');setHeaderMood('thinking');scrollBottom();
  try{
    var r=await fetch('/conversations/'+encodeURIComponent(sentId)+'/messages',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({content:option,mode:currentMode||'bestie'})}));
    var data=null;try{data=await r.json();}catch(_){}
    typing.classList.remove('show');setHeaderMood(null);
    if(r.ok&&currentId===sentId){
      var qReplies=data&&data.replies&&Array.isArray(data.replies)?data.replies:null;
      if(qReplies&&qReplies.length>0){qReplies.forEach(function(part,i){addBubble('assistant',part.content||'',true,i===qReplies.length-1?(data.quick_replies||null):null,false,{style:part.style||'final'});});}else{addBubble('assistant',(data&&data.reply)!==undefined?data.reply:'',true,data&&data.quick_replies);}
      playDoneThinkingSound();
      fetch('/conversations/'+encodeURIComponent(sentId),mergeApiHeaders({})).then(function(r2){return r2.ok?r2.json():null;}).then(async function(c){if(!c||!c.messages||currentId!==sentId)return;currentMessages=c.messages;currentFeedback={};try{var fr=await fetch('/conversations/'+encodeURIComponent(sentId)+'/feedback',mergeApiHeaders({}));if(fr.ok){var fd=await fr.json();if(fd&&fd.feedback&&typeof fd.feedback==='object'){for(var k in fd.feedback){var i=parseInt(k,10);if(!isNaN(i))currentFeedback[i]=fd.feedback[k];}}}}catch(_){}
if(currentId!==sentId)return;renderMessages(currentMessages,false,'mobile');scrollBottom();}).catch(function(){});}
    else if(!r.ok&&currentId===sentId){addBubble('assistant',(data&&data.detail)?String(data.detail):'Error '+r.status,true);if(r.status===404)fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r2){return r2.ok?r2.json():null;}).then(function(d){if(d&&Array.isArray(d.conversations)){mobileConvos=d.conversations;renderSidebar();}}).catch(function(){});}
  }catch(e){typing.classList.remove('show');setHeaderMood(null);if(currentId===sentId)addBubble('assistant','Connection error: '+(e.message||e),true);}
  finally{sendInProgress=false;sendBtn.disabled=false;drainPendingQueue(sentId);}
}
function renderMessages(messages,ro,src,opts){
  opts=opts||{};
  clearChat();
  if(!messages||!messages.length){showEmpty()}
  else{
    var frag=document.createDocumentFragment();
    var isGroup=opts.group===true;
    var canEdit=src==='mobile'&&!ro&&!isGroup;
    var lastAssistantIdx=-1;for(var i=messages.length-1;i>=0;i--){var r=(messages[i].role||'').toLowerCase();if(r==='assistant'){lastAssistantIdx=i;break;}}
    messages.forEach(function(m,idx){
      var role=(m.role||'').toLowerCase();
      if(role!=='user'&&role!=='assistant')role='assistant';
      var sender=m.sender||(role==='assistant'?'locus':getCurrentUser());
      var senderLabel=sender==='locus'?'Locus':(sender||'You');
      var wrap=document.createElement('div');wrap.className='msg-wrap '+(role==='user'?'user':'assistant')+(isGroup?' group-msg':'')+(role==='assistant'&&m.style==='thinking'?' thinking-msg':'');
      if(isGroup){var senderRow=document.createElement('div');senderRow.className='msg-row msg-sender-row';senderRow.innerHTML='<span class="msg-sender-label">'+escapeHtml(senderLabel)+'</span>';wrap.appendChild(senderRow);}
      var row=document.createElement('div');row.className='msg-row';
      var av=document.createElement('div');
      if(role==='user'){av.className='msg-av user-av';var ui=document.createElement('img');ui.src=currentUserAvatarUrl;ui.alt='';ui.style.cssText='width:64px;height:64px;max-width:64px;max-height:64px;object-fit:contain;display:block;flex-shrink:0';ui.onerror=function(){av.textContent='\u273F';};av.appendChild(ui);}
      else{av.className='msg-av';var ai=document.createElement('img');ai.src='/locus_avatar.svg';ai.alt='';ai.style.cssText='width:40px;height:40px;max-width:40px;max-height:40px;object-fit:contain;display:block;flex-shrink:0';ai.onerror=function(){this.src='/chat_icon.png';this.onerror=function(){this.src='/icon.svg';};};av.appendChild(ai);}
      var bubble=document.createElement('div');bubble.className='bubble';
      var content=m.content||'';
      if(role==='user'){
        var ip=m.image_path;
        if(ip){
          var iw=document.createElement('div');iw.className='chat-img-wrap';
          var im=document.createElement('img');im.className='chat-img';im.src='/api/chat_image?path='+encodeURIComponent(ip);im.alt='Sent photo';im.loading='lazy';
          im.onclick=function(){openImageLightbox(im.src);};
          im.onerror=function(){iw.style.display='none';};
          iw.appendChild(im);bubble.appendChild(iw);
        }
        var fileNames=m.file_names;
        if(fileNames&&fileNames.length)appendChatFileIcons(bubble,fileNames);
        var displayContent=content;
        if(fileNames&&fileNames.length)displayContent=(content||'').replace(/\n?,?\s*\[File: [^\]]+\]/g,'').replace(/\s*\[Image:[^\]]*\]/g,'').trim();
        var prevMsg=idx>0?messages[idx-1]:null;
        var prevOptions=(prevMsg&&(prevMsg.role||'').toLowerCase()==='assistant'&&prevMsg.quick_replies&&Array.isArray(prevMsg.quick_replies)&&prevMsg.quick_replies.length)?prevMsg.quick_replies:DEFAULT_FOLLOWUP_SUGGESTIONS;
        if(prevOptions.indexOf((displayContent||'').trim())!==-1)displayContent='\u21B3 Chose: '+(displayContent||'').trim();
        bubble.appendChild(document.createTextNode(displayContent||''));
      }else{
        var aip=m.generated_image_path||m.image_path;
        if(aip){var aiw=document.createElement('div');aiw.className='chat-img-wrap';var aim=document.createElement('img');aim.className='chat-img';aim.src='/api/chat_image?path='+encodeURIComponent(aip);aim.alt='Generated image';aim.loading='lazy';aim.onclick=function(){openImageLightbox(aim.src);};aim.onerror=function(){aiw.style.display='none';};aiw.appendChild(aim);bubble.appendChild(aiw);}
        if(m.style==='thinking'){
          var details=document.createElement('details');details.className='thinking-details';
          var summary=document.createElement('summary');summary.className='thinking-summary';summary.textContent='Thought';
          var readout=document.createElement('div');readout.className='thinking-readout';readout.innerHTML=(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');
          details.appendChild(summary);details.appendChild(readout);bubble.appendChild(details);
        }else{
          var textEl=document.createElement('div');textEl.className='bubble-text';
          var parsed=parsePlanBlock(content);
          var html='';
          if(parsed&&parsed.plan){
            try{
              if(parsed.before)html+=(window.marked?marked.parse(parsed.before,{breaks:true,gfm:true}):(parsed.before||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>'))+'<br>';
              html+='<details class="assistant-plan"><summary>Plan / steps</summary><div class="bubble-text">'+(window.marked?marked.parse(parsed.plan,{breaks:true,gfm:true}):(parsed.plan||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>'))+'</div></details>';
              if(parsed.after)html+='<br>'+(window.marked?marked.parse(parsed.after,{breaks:true,gfm:true}):(parsed.after||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>'));
            }catch(e){html=(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');}
          }else{
            try{html=window.marked?marked.parse(content,{breaks:true,gfm:true}):(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');}catch(e){}
          }
          if(html)textEl.innerHTML=html;else textEl.textContent=content||'';
          bubble.appendChild(textEl);
        }
      }
      row.appendChild(av);row.appendChild(bubble);wrap.appendChild(row);
      var addCopyBtn=function(actWrapEl,wrapEl){
        var copyBtn=document.createElement('button');copyBtn.type='button';copyBtn.className='copy-msg-btn';copyBtn.setAttribute('aria-label','Copy message');
        copyBtn.innerHTML=ICON_COPY_NEON;
        copyBtn.addEventListener('click',function(){var w=copyBtn.closest('.msg-wrap');copyMessageAndFeedback(copyBtn,w?w.querySelector('.bubble'):null);});
        actWrapEl.appendChild(copyBtn);
      };
      if(role==='user'&&canEdit){
        var actRow=document.createElement('div');actRow.className='msg-row msg-actions';
        var actSpacer=document.createElement('div');actSpacer.className='msg-av';actSpacer.setAttribute('aria-hidden','true');
        var actWrap=document.createElement('div');actWrap.className='bubble-actions';
        var btnWrap=document.createElement('div');btnWrap.className='edit-msg-btn-wrap';
        var editBtn=document.createElement('button');editBtn.type='button';editBtn.className='edit-msg-btn';editBtn.setAttribute('aria-label','Edit and start new thread');editBtn.dataset.msgIndex=String(idx);
        editBtn.innerHTML=ICON_BUBBLE_NEON;
        editBtn.addEventListener('click',function(){startEditMessage(wrap,idx,content,editBtn,btnWrap);});
        btnWrap.appendChild(editBtn);actWrap.appendChild(btnWrap);
        addCopyBtn(actWrap,wrap);
        actRow.appendChild(actSpacer);actRow.appendChild(actWrap);wrap.appendChild(actRow);
      }else{
        var actRow=document.createElement('div');actRow.className='msg-row msg-actions';
        var actSpacer=document.createElement('div');actSpacer.className='msg-av';actSpacer.setAttribute('aria-hidden','true');
        var actWrap=document.createElement('div');actWrap.className='bubble-actions';
        if(role==='assistant'&&canEdit){
          var variants=m.variants;var sel=typeof m.selected==='number'?m.selected:0;
          if(Array.isArray(variants)&&variants.length>1){
            var vWrap=document.createElement('div');vWrap.className='variant-picker';
            var vPrev=document.createElement('button');vPrev.type='button';vPrev.className='variant-picker-btn';vPrev.setAttribute('aria-label','Previous message');vPrev.textContent='\u2039';
            var vLabel=document.createElement('span');vLabel.className='variant-picker-label';vLabel.textContent=(sel+1)+'/'+variants.length;
            var vNext=document.createElement('button');vNext.type='button';vNext.className='variant-picker-btn';vNext.setAttribute('aria-label','Next message');vNext.textContent='\u203A';
            vPrev.addEventListener('click',function(){if(sel>0)selectVariant(idx,sel-1,vWrap,bubble);});
            vNext.addEventListener('click',function(){if(sel<variants.length-1)selectVariant(idx,sel+1,vWrap,bubble);});
            vWrap.appendChild(vPrev);vWrap.appendChild(vLabel);vWrap.appendChild(vNext);actWrap.appendChild(vWrap);
          }
          var upBtn=document.createElement('button');upBtn.type='button';upBtn.className='feedback-btn feedback-up';upBtn.setAttribute('aria-label','Good reply');upBtn.textContent='\uD83D\uDC4D';
          upBtn.dataset.idx=String(idx);
          upBtn.addEventListener('click',function(){sendFeedback(Number(upBtn.dataset.idx),'up',upBtn);});
          var downBtn=document.createElement('button');downBtn.type='button';downBtn.className='feedback-btn feedback-down';downBtn.setAttribute('aria-label','Poor reply');downBtn.textContent='\uD83D\uDC4E';
          downBtn.dataset.idx=String(idx);
          downBtn.addEventListener('click',function(){sendFeedback(Number(downBtn.dataset.idx),'down',downBtn);});
          var fb=currentFeedback[idx];
          if(fb==='up'){upBtn.classList.add('feedback-sent');upBtn.classList.add('feedback-up');upBtn.setAttribute('aria-label','Liked');}
          else if(fb==='down'){downBtn.classList.add('feedback-sent');downBtn.classList.add('feedback-down');downBtn.setAttribute('aria-label','Noted');}
          actWrap.appendChild(upBtn);actWrap.appendChild(downBtn);
        }
        addCopyBtn(actWrap,wrap);
        actRow.appendChild(actSpacer);actRow.appendChild(actWrap);wrap.appendChild(actRow);
      }
      var nextMsg=idx+1<messages.length?messages[idx+1]:null;
      var nextContent=nextMsg&&(nextMsg.role||'').toLowerCase()==='user'?(nextMsg.content||'').trim():'';
      var optionsForThisAssistant=(m.quick_replies&&Array.isArray(m.quick_replies)&&m.quick_replies.length>0)?m.quick_replies:DEFAULT_FOLLOWUP_SUGGESTIONS;
      var followedByChoice=role==='assistant'&&nextMsg&&(nextMsg.role||'').toLowerCase()==='user'&&optionsForThisAssistant.indexOf(nextContent)!==-1;
      if(role==='assistant'&&followedByChoice){
        var qrRowUsed=document.createElement('div');qrRowUsed.className='msg-row quick-replies';
        var qrSpacerUsed=document.createElement('div');qrSpacerUsed.className='msg-av';qrSpacerUsed.setAttribute('aria-hidden','true');
        var qrWrapUsed=document.createElement('div');qrWrapUsed.className='quick-reply-wrap quick-replies-used';
        optionsForThisAssistant.forEach(function(label){
          var btn=document.createElement('button');btn.type='button';btn.className='quick-reply-btn used'+(label===nextContent?' chosen':'');btn.textContent=label;btn.dataset.option=label;btn.disabled=true;
          qrWrapUsed.appendChild(btn);
        });
        qrRowUsed.appendChild(qrSpacerUsed);qrRowUsed.appendChild(qrWrapUsed);wrap.appendChild(qrRowUsed);
      }else if(role==='assistant'&&idx===lastAssistantIdx&&canEdit){
        var qrRow=document.createElement('div');qrRow.className='msg-row quick-replies';
        var qrSpacer=document.createElement('div');qrSpacer.className='msg-av';qrSpacer.setAttribute('aria-hidden','true');
        var qrWrap=document.createElement('div');qrWrap.className='quick-reply-wrap';
        var activeOptions=(m.quick_replies&&Array.isArray(m.quick_replies)&&m.quick_replies.length>0)?m.quick_replies:DEFAULT_FOLLOWUP_SUGGESTIONS;
        activeOptions.forEach(function(label){
          var btn=document.createElement('button');btn.type='button';btn.className='quick-reply-btn';btn.textContent=label;btn.dataset.option=label;
          btn.addEventListener('click',function(){sendQuickReply(label,btn);});
          qrWrap.appendChild(btn);
        });
        qrRow.appendChild(qrSpacer);qrRow.appendChild(qrWrap);wrap.appendChild(qrRow);
      }
      frag.appendChild(wrap);
    });
    chatArea.insertBefore(frag,typing);
    if(opts.cascade){
      try{
        var wraps=Array.from(chatArea.querySelectorAll('.msg-wrap'));
        var maxAnimated=80;
        var cascadeStaggerMs=58;
        var cascadeDuration='0.24s';
        wraps.forEach(function(w,i){
          if(i>=maxAnimated)return;
          w.style.opacity='0';
          w.style.transform='translateY(8px)';
          w.style.transition='opacity '+cascadeDuration+' ease-out, transform '+cascadeDuration+' ease-out';
        });
        wraps.forEach(function(w,i){
          if(i>=maxAnimated)return;
          setTimeout(function(){
            w.style.opacity='1';
            w.style.transform='translateY(0)';
          },i*cascadeStaggerMs);
        });
      }catch(_){}
    }
  }
  if(opts.group){updateCopyConvoButtonVisibility();scrollBottom();return;}
  var convo=allConvos().find(function(c){return c.id===currentId&&c.source===currentSrc});
  if(convo){hdrName.textContent=convo.title||'Locus \u2665';hdrSub.textContent=srcLabel(convo.source)+' chat'}
  else{hdrName.textContent='Locus \u2665';hdrSub.textContent='Locus \u00B7 same workspace as your PC'}
  if(ro){
roHint.style.display='flex';
var srcName={continue:'VS Code',grok:'Grok',cursor:'Cursor'}[src]||src;
roHint.innerHTML='<span>'+srcName+' chat \u2014 read only</span>'
  +'<button id="forkBtn">\u25B6\uFE0F Continue with Locus</button>';
document.getElementById('forkBtn').addEventListener('click',forkConvo);
msgInput.disabled=true;sendBtn.style.display='none';
  }else{roHint.style.display='none';msgInput.disabled=false;sendBtn.style.display='flex'}
  currentRO=ro;updateCopyConvoButtonVisibility();scrollBottom();
}
function startEditMessage(wrap,msgIndex,originalContent,editBtn,btnWrap){
  var row=wrap.querySelector('.msg-row');
  var bubble=row?row.querySelector('.bubble'):null;
  var actRow=wrap.querySelector('.msg-actions');
  if(!bubble||!row)return;
  function openEditor(){
    var container=document.createElement('div');container.className='edit-msg-inline';
    var ta=document.createElement('textarea');ta.className='edit-msg-textarea';ta.rows=4;ta.value=originalContent;ta.placeholder='Edit your message...';
    var btnRow=document.createElement('div');btnRow.className='edit-msg-buttons';
    var cancelBtn=document.createElement('button');cancelBtn.type='button';cancelBtn.className='edit-msg-cancel';cancelBtn.setAttribute('aria-label','Cancel');
    cancelBtn.innerHTML='<img src="'+EDIT_MSG_HORNS_URL+'" alt="" class="edit-msg-icon-horns" width="24" height="24">';
    var forkBtn=document.createElement('button');forkBtn.type='button';forkBtn.className='edit-msg-fork';forkBtn.setAttribute('aria-label','Start new thread from here');
    forkBtn.innerHTML=ICON_BUBBLES_POP_NEON;
    var sendHereBtn=document.createElement('button');sendHereBtn.type='button';sendHereBtn.className='edit-msg-send-here';sendHereBtn.setAttribute('aria-label','Send again here (edit in place)');
    sendHereBtn.innerHTML='<img src="'+EDIT_MSG_PENCIL_URL+'" alt="" class="edit-msg-icon-pencil" width="22" height="22">';
    function closeEdit(){
      if(container.parentNode)container.parentNode.removeChild(container);
      if(actRow)actRow.style.display='';
      bubble.style.display='';
      var eb=wrap.querySelector('.edit-msg-btn');
      if(eb){eb.classList.remove('popping');eb.disabled=false;eb.innerHTML=ICON_BUBBLE_NEON;}
    }
    cancelBtn.addEventListener('click',function(){if(typeof poofDismiss==='function')poofDismiss(container,closeEdit);else closeEdit();});
    forkBtn.addEventListener('click',function(){
      var edited=ta.value.trim();
      if(!edited){closeEdit();return;}
      forkBtn.disabled=true;forkBtn.setAttribute('aria-label','Creating thread...');
      forkAndSendEdited(msgIndex,edited).then(closeEdit).catch(function(e){forkBtn.disabled=false;forkBtn.setAttribute('aria-label','Start new thread from here');alert('Could not start thread: '+(e.message||e));});
    });
    sendHereBtn.addEventListener('click',function(){
      var edited=ta.value.trim();
      if(!edited){closeEdit();return;}
      sendHereBtn.classList.add('sparkle');setTimeout(function(){sendHereBtn.classList.remove('sparkle');},400);
      sendHereBtn.disabled=true;sendHereBtn.setAttribute('aria-label','Sending...');
      editAndContinueInPlace(msgIndex,edited).then(closeEdit).catch(function(e){sendHereBtn.disabled=false;sendHereBtn.setAttribute('aria-label','Send again here (edit in place)');alert('Could not send: '+(e.message||e));});
    });
    btnRow.appendChild(cancelBtn);btnRow.appendChild(sendHereBtn);btnRow.appendChild(forkBtn);
    container.appendChild(ta);container.appendChild(btnRow);
    if(actRow)actRow.style.display='none';
    bubble.style.display='none';
    row.appendChild(container);
    ta.focus();
  }
  if(editBtn&&btnWrap){
    editBtn.classList.add('popping');editBtn.disabled=true;
    editBtn.innerHTML=ICON_BUBBLES_POP_NEON;
    sparkleBurst(btnWrap);
    setTimeout(openEditor,380);
  }else{openEditor();}
}
async function forkAndSendEdited(msgIndex,editedContent){
  if(!currentId||currentSrc!=='mobile')throw new Error('No conversation');
  var r=await fetch('/conversations/'+encodeURIComponent(currentId)+'/fork_branch',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message_index:msgIndex,content:editedContent,mode:currentMode||'bestie',branch_index:currentBranchIndex})}));
  if(!r.ok){var d=null;try{d=await r.json();}catch(_){}throw new Error((d&&d.detail)||'fork failed');}
  var data=await r.json();
  currentBranchIndex=data.branch_index!==undefined?data.branch_index:1;
  branchCount=data.branch_count||1;
  currentMessages=data.messages||[];
  currentFeedback={};
  renderMessages(currentMessages,false,'mobile');
  updateThreadSwitcher();
  if(data.title){var idx=mobileConvos.findIndex(function(c){return c.id===currentId});if(idx>=0){mobileConvos[idx]=Object.assign({},mobileConvos[idx],{title:data.title,updated_at:data.updated_at||new Date().toISOString()});renderSidebar();}}
  closeSidebar();if(msgInput)msgInput.focus();playDoneThinkingSound();
}
async function sendGroupMessage(){
  var text=msgInput.value.trim();
  var hasAttach=pendingAttachments.length>0;
  if(!text&&!hasAttach)return;
  var fileList=pendingFiles();var imgAtt=pendingImage();
  var displayText=text||(imgAtt?'[Image]':(fileList.length?fileList.length===1?'[File: '+fileList[0].name+']':'['+fileList.length+' files]':'[File]'));
  if(displayText.length>120)displayText=displayText.slice(0,117)+'...';
  var imgToSend=imgAtt?imgAtt.imageBase64:null;
  var textParts=[text||''];
  pendingAttachments.forEach(function(a){if(a.type==='file'&&a.fileText!=null)textParts.push('[Attached: '+a.name+']:\n'+a.fileText);});
  var contentToSend=textParts.join('\n\n').trim()||'(no text)';
  var fileB64ToSend=null,fileMimeToSend=null,fileB64Name='';
  var firstPdf=pendingAttachments.find(function(a){return a.type==='file'&&a.fileBase64;});
  if(firstPdf){fileB64ToSend=firstPdf.fileBase64;fileMimeToSend=firstPdf.fileMime||'application/pdf';fileB64Name=firstPdf.name;}
  pendingAttachments=[];hideAttachPreview();
  msgInput.value='';msgInput.placeholder='Message...';autoResize();updateContextIndicator();
  typing.classList.add('show');setHeaderMood('thinking');sendBtn.disabled=true;scrollBottom();
  try{
    var body={content:contentToSend,want_reply:true,image_base64:imgToSend||null};
    if(fileB64ToSend){body.file_base64=fileB64ToSend;body.file_name=fileB64Name;body.file_mime=fileMimeToSend||'application/pdf';}
    var r=await fetch('/api/group_chat/messages',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}));
    var data=null;try{data=await r.json();}catch(_){}
    typing.classList.remove('show');setHeaderMood(null);sendBtn.disabled=false;
    if(!r.ok){if(data&&data.detail)addBubble('assistant',String(data.detail),true);else addBubble('assistant','Could not send. Try again.',true);return;}
    currentMessages=data.messages||[];
    renderMessages(currentMessages,false,'mobile',{group:true});
    if(data.reply)playDoneThinkingSound();
    scrollBottom();
  }catch(e){typing.classList.remove('show');setHeaderMood(null);sendBtn.disabled=false;addBubble('assistant','Connection error: '+(e.message||e),true);}
}
async function editAndContinueInPlace(msgIndex,editedContent){
  if(!currentId||currentSrc!=='mobile')throw new Error('No conversation');
  var r=await fetch('/conversations/'+encodeURIComponent(currentId)+'/edit_and_continue',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message_index:msgIndex,content:editedContent,branch_index:currentBranchIndex})}));
  if(!r.ok){var d=null;try{d=await r.json();}catch(_){}throw new Error((d&&d.detail)||'edit failed');}
  var data=await r.json();
  if(data&&data.messages){currentMessages=data.messages;currentFeedback={};try{var fr=await fetch('/conversations/'+encodeURIComponent(currentId)+'/feedback',mergeApiHeaders({}));if(fr.ok){var fd=await fr.json();if(fd&&fd.feedback&&typeof fd.feedback==='object'){for(var k in fd.feedback){var i=parseInt(k,10);if(!isNaN(i))currentFeedback[i]=fd.feedback[k];}}}}catch(_){}
    renderMessages(currentMessages,false,'mobile');scrollBottom();}
  var idx=mobileConvos.findIndex(function(c){return c.id===currentId});if(idx>=0&&data&&data.title){mobileConvos[idx]=Object.assign({},mobileConvos[idx],{title:data.title,updated_at:data.updated_at});renderSidebar();}
}
function showFeedbackToast(label){
  var live=document.getElementById('feedbackToastLive');
  if(!live){
    live=document.createElement('div');live.id='feedbackToastLive';live.setAttribute('role','status');live.setAttribute('aria-live','polite');
    live.style.cssText='position:fixed;bottom:90px;left:50%;transform:translateX(-50%);padding:8px 14px;border-radius:10px;background:var(--pink,#ff7ad9);color:#111;font-size:13px;font-weight:600;z-index:99999;opacity:0;transition:opacity .2s';
    document.body.appendChild(live);
  }
  live.textContent=label;live.style.opacity='1';
  clearTimeout(live._hide);live._hide=setTimeout(function(){live.style.opacity='0';},1200);
}
async function selectVariant(msgIndex,variantIndex,pickerWrap,bubbleEl){
  if(!currentId||currentSrc!=='mobile')return;
  var r=await fetch('/conversations/'+encodeURIComponent(currentId)+'/messages/'+msgIndex+'/select_variant',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({variant_index:variantIndex,branch_index:(typeof currentBranchIndex==='number'?currentBranchIndex:0)})}));
  if(!r.ok)return;
  var data=null;try{data=await r.json();}catch(_){}
  if(!data||!currentMessages||msgIndex>=currentMessages.length)return;
  var m=currentMessages[msgIndex];if(!m.variants||variantIndex<0||variantIndex>=m.variants.length)return;
  m.selected=variantIndex;m.content=m.variants[variantIndex];
  if(pickerWrap){var lbl=pickerWrap.querySelector('.variant-picker-label');if(lbl)lbl.textContent=(variantIndex+1)+'/'+m.variants.length;}
  if(bubbleEl){
    var textEl=bubbleEl.querySelector('.bubble-text');var content=m.content||'';
    if(textEl){
      var html='';var parsed=parsePlanBlock(content);
      if(parsed&&parsed.plan){
        try{
          if(parsed.before)html+=(window.marked?marked.parse(parsed.before,{breaks:true,gfm:true}):(parsed.before||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>'))+'<br>';
          html+='<details class="assistant-plan"><summary>Plan / steps</summary><div class="bubble-text">'+(window.marked?marked.parse(parsed.plan,{breaks:true,gfm:true}):(parsed.plan||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>'))+'</div></details>';
          if(parsed.after)html+='<br>'+(window.marked?marked.parse(parsed.after,{breaks:true,gfm:true}):(parsed.after||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>'));
        }catch(e){html=(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');}
      }else{
        try{
          if(window.marked){html=marked.parse(content,{breaks:true,gfm:true});}
          else{html=(content||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');}
        }catch(e){}
      }
      if(html)textEl.innerHTML=html;else textEl.textContent=content||'';
    }
  }
}
function sendFeedback(msgIndex,rating,btn){
  if(!currentId||currentSrc!=='mobile')return;
  if(btn&&btn.classList.contains('feedback-sent')&&currentFeedback[msgIndex]===rating)return;
  fetch('/conversations/'+encodeURIComponent(currentId)+'/feedback',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message_index:msgIndex,rating:rating,branch_index:currentBranchIndex||0})}))
    .then(function(r){
      if(r.ok){
        currentFeedback[msgIndex]=rating;
        if(btn){
          btn.classList.add('feedback-sent');
          btn.setAttribute('aria-label',rating==='up'?'Liked':'Noted');
          var actRow=btn.closest('.msg-actions');
          if(actRow){
            var other=actRow.querySelector(rating==='up'?'.feedback-down':'.feedback-up');
            if(other){other.classList.remove('feedback-sent');other.setAttribute('aria-label',rating==='up'?'Poor reply':'Good reply');}
          }
        }
        showFeedbackToast(rating==='up'?'Liked':'Noted');
      }
    })
    .catch(function(){});
}
function scrollBottom(){requestAnimationFrame(function(){chatArea.scrollTop=chatArea.scrollHeight})}

/* ── Load ── */
async function loadAll(){
  /* Fast path: load only mobile conversations first so chat appears immediately (iPhone/slow networks). */
  try{
    var r=await fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'}));
    if(!r.ok&&r.status===403){ try{ var j=await r.json(); if(j&&j.detail==='account_locked')showAccountLocked(); }catch(_){} return; }
    var data=r.ok?await r.json():null;
    mobileConvos=(data&&data.conversations)?data.conversations:[];
    if(!mobileConvos.length&&!r.ok){ var r2=await fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})); if(r2.ok){ var d2=await r2.json(); mobileConvos=(d2&&d2.conversations)?d2.conversations:[]; } }
  }catch(e){ mobileConvos=[]; }
  if(!mobileConvos.length){
    try{ var cr=await fetch('/conversations',mergeApiHeaders({method:'POST'})); if(cr.ok)mobileConvos=[(await cr.json())]; }catch(_){}
  }
  if(!currentId&&mobileConvos.length){ currentId=mobileConvos[0].id; currentSrc='mobile'; }
  renderSidebar();
  if(currentId){ await loadConvo(currentId,currentSrc); } else { clearChat(); showEmpty(); currentBranchIndex=0; branchCount=1; updateThreadSwitcher(); }
  /* Load other sources in background only when signed in (VS Code/Grok/Cursor hidden when signed out). */
  var promises=[fetch('/conversations/archive',mergeApiHeaders({}))];
  if(signedInUser){
    promises.push(fetch('/continue/conversations'),fetch('/grok/conversations'),fetch('/cursor/conversations'));
  }else{
    continueConvos=[];grokConvos=[];cursorConvos=[];
  }
  Promise.allSettled(promises).then(async function(results){
    var i=0;
    try{ if(results[i].status==='fulfilled'&&results[i].value.ok)archivedConvos=(await results[i].value.json()).conversations||[]; }catch(_){}
    i++;
    if(signedInUser&&results[i]){
      try{ if(results[i].status==='fulfilled'&&results[i].value.ok)continueConvos=(await results[i].value.json()).conversations||[]; }catch(_){}
      i++;
      try{ if(results[i].status==='fulfilled'&&results[i].value.ok)grokConvos=(await results[i].value.json()).conversations||[]; }catch(_){}
      i++;
      try{ if(results[i].status==='fulfilled'&&results[i].value.ok)cursorConvos=(await results[i].value.json()).conversations||[]; }catch(_){}
    }
    renderSidebar();
  }).catch(function(){});
}
async function loadConvo(id,src){
  currentId=id;currentSrc=src||'mobile';renderSidebar();
  hideEngagementBanner();
  currentFeedback={};
  if(src==='mobile'){
    try{
      var fr=await fetch('/conversations/'+encodeURIComponent(id)+'/feedback',mergeApiHeaders({}));
      if(fr.ok){var fd=await fr.json();if(fd&&fd.feedback&&typeof fd.feedback==='object'){for(var k in fd.feedback){var idx=parseInt(k,10);if(!isNaN(idx))currentFeedback[idx]=fd.feedback[k];}}}
    }catch(_){}
  }
  var ep={mobile:'/conversations/'+encodeURIComponent(id),continue:'/continue/conversations/'+encodeURIComponent(id),grok:'/grok/conversations/'+encodeURIComponent(id),cursor:'/cursor/conversations/'+encodeURIComponent(id)};
  if(src==='mobile'){ep.mobile=ep.mobile+'?branch='+currentBranchIndex;}
  if(src==='grok'){ep.grok=ep.grok+'?branch='+currentBranchIndex;}
  try{
var r=await fetch(ep[src]||ep.mobile,(src==='mobile')?mergeApiHeaders({}):{});
if(!r.ok){clearChat();showEmpty();currentMessages=[];currentConvoTitle='';currentBranchIndex=0;branchCount=1;updateCopyConvoButtonVisibility();updateThreadSwitcher();return;}
var c=await r.json();
currentMessages=c.messages||[];currentConvoTitle=(src==='mobile'?(c.title||'Chat'):(c.title||'Grok chat'));
if(src==='mobile'||src==='grok'){branchCount=c.branch_count||1;currentBranchIndex=typeof c.branch_index==='number'?c.branch_index:0;}else{branchCount=1;currentBranchIndex=0;}
var branchCountNow=c.branch_count||1;
var cascadeOpts=(branchCountNow>1)?{cascade:true}:{};
renderMessages(c.messages||[],src!=='mobile',src,cascadeOpts);
updateThreadSwitcher();
recordEngagementAndMaybePrompt(id,src);
restoreDraftForConvo(id,src);
var searchQ=(searchResults!==null&&sbSearch&&(sbSearch.value||'').trim())?(sbSearch.value||'').trim():'';
if(searchQ){setTimeout(function(){scrollToAndHighlightFirstMatch(searchQ);},120);}
  }catch(e){console.error(e);clearChat();showEmpty();currentMessages=[];currentConvoTitle='';currentBranchIndex=0;branchCount=1;updateCopyConvoButtonVisibility();updateThreadSwitcher();}
}
function hideEngagementBanner(){
  var b=document.getElementById('engagementBanner');if(b){b.classList.remove('show');b.style.display='none';}
}
function recordEngagementAndMaybePrompt(convId,src){
  fetch('/conversations/engagement/record',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({source:src,id:convId})})).then(function(r){return r.json();}).then(function(data){
    if(!data.suggest_important)return;
    var banner=document.getElementById('engagementBanner');if(!banner)return;
    var text=banner.querySelector('.engagement-banner-text');var yesBtn=banner.querySelector('.engagement-banner-yes');var noBtn=banner.querySelector('.engagement-banner-no');
    if(text)text.textContent="You've returned to this chat a few times. Star this chat?";
    if(yesBtn){yesBtn.onclick=function(){yesBtn.disabled=true;fetch('/conversations/engagement/mark_important',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({source:src,id:convId,important:true})})).then(function(){hideEngagementBanner();if(src==='mobile'){var idx=mobileConvos.findIndex(function(c){return c.id===convId});if(idx>=0){mobileConvos[idx]=Object.assign({},mobileConvos[idx],{important:true});renderSidebar();}};});};}
    if(noBtn){noBtn.onclick=function(){hideEngagementBanner();};}
    banner.classList.add('show');banner.style.display='flex';
  }).catch(function(){});
}
function switchConvo(id,src){currentId=id;currentSrc=src;loadConvo(id,src)}
function scrollToAndHighlightFirstMatch(term){
  if(!term||!chatArea)return;
  var bubbles=chatArea.querySelectorAll('.bubble');
  var lower=term.toLowerCase();
  for(var i=0;i<bubbles.length;i++){
    var bubble=bubbles[i];
    var text=(bubble.textContent||'');
    var idx=text.toLowerCase().indexOf(lower);
    if(idx===-1)continue;
    bubble.querySelectorAll('mark.search-highlight').forEach(function(m){var t=document.createTextNode(m.textContent);m.parentNode.replaceChild(t,m);});
    var walker=document.createTreeWalker(bubble,NodeFilter.SHOW_TEXT,null,false);
    var node;
    while((node=walker.nextNode())){
      var t=node.textContent;
      var pos=t.toLowerCase().indexOf(lower);
      if(pos===-1)continue;
      var before=t.slice(0,pos);
      var match=t.slice(pos,pos+term.length);
      var after=t.slice(pos+term.length);
      var frag=document.createDocumentFragment();
      if(before)frag.appendChild(document.createTextNode(before));
      var mark=document.createElement('mark');
      mark.className='search-highlight';
      mark.textContent=match;
      frag.appendChild(mark);
      if(after)frag.appendChild(document.createTextNode(after));
      node.parentNode.replaceChild(frag,node);
      break;
    }
    bubble.scrollIntoView({behavior:'smooth',block:'center'});
    return;
  }
}

/* ── Send ── */
function fileToBase64(file,cb){
  var r=new FileReader();
  r.onload=function(){var s=r.result;if(s.indexOf('base64,')>=0)s=s.slice(s.indexOf('base64,')+7);cb(s);};
  r.readAsDataURL(file);
}
var TEXT_EXT=/\.(txt|md|json|csv|log|html|xml|jsonl)$/i;
function updateChatModeUI(){var area=document.getElementById('inputArea');var lbl=document.getElementById('chatModeLabel');var rolesBtn=document.getElementById('rolesMenuBtn');var modeLabel=currentMode==='therapist'?'Therapist':currentMode==='learning'?'Learning':'Bestie';if(area){area.classList.remove('mode-bestie','mode-therapist','mode-learning');area.classList.add('mode-'+currentMode);area.setAttribute('data-mode',currentMode);}if(lbl)lbl.textContent=modeLabel;if(rolesBtn)rolesBtn.textContent='Roles · '+modeLabel;try{localStorage.setItem(CHAT_MODE_KEY,currentMode);}catch(e){}}
function updateThreadSwitcher(){var wrap=document.getElementById('threadSwitcher');if(!wrap)return;if(currentSrc==='mobile'){wrap.style.display='none';return;}if(branchCount>1){wrap.style.display='flex';var lbl=wrap.querySelector('.thread-switcher-label');var prev=wrap.querySelector('.thread-switcher-prev');var next=wrap.querySelector('.thread-switcher-next');if(lbl)lbl.textContent=(currentBranchIndex+1)+'/'+branchCount;if(prev){prev.disabled=currentBranchIndex<=0;prev.setAttribute('aria-label','Thread '+(currentBranchIndex)+' of '+branchCount)}if(next){next.disabled=currentBranchIndex>=branchCount-1;next.setAttribute('aria-label','Thread '+(currentBranchIndex+2)+' of '+branchCount)}}else{wrap.style.display='none'}}
function openAttachMenu(){var pop=document.getElementById('attachMenuPopover');if(!pop)return;pop.classList.add('open');if(attachImgBtn)attachImgBtn.setAttribute('aria-expanded','true');closeRolesSubmenu();var rolesBtn=document.getElementById('rolesMenuBtn');if(rolesBtn)rolesBtn.textContent='Roles · '+(currentMode==='therapist'?'Therapist':currentMode==='learning'?'Learning':'Bestie');syncRolesSubmenuActive();}
function closeAttachMenu(){var pop=document.getElementById('attachMenuPopover');if(pop)pop.classList.remove('open');if(attachImgBtn)attachImgBtn.setAttribute('aria-expanded','false');closeRolesSubmenu();}
function openRolesSubmenu(){var sub=document.getElementById('rolesSubmenuPopover');if(sub){sub.classList.add('open');syncRolesSubmenuActive();var btn=document.getElementById('rolesMenuBtn');if(btn)btn.setAttribute('aria-expanded','true');}}
function closeRolesSubmenu(){var sub=document.getElementById('rolesSubmenuPopover');if(sub)sub.classList.remove('open');var btn=document.getElementById('rolesMenuBtn');if(btn)btn.setAttribute('aria-expanded','false');}
function syncRolesSubmenuActive(){var sub=document.getElementById('rolesSubmenuPopover');if(!sub)return;sub.querySelectorAll('.attach-menu-item.mode-btn').forEach(function(b){var m=b.dataset.mode||'bestie';b.classList.toggle('active',m===currentMode);b.setAttribute('aria-pressed',m===currentMode?'true':'false');});}
if(attachImgBtn){
  attachImgBtn.addEventListener('click',function(e){e.stopPropagation();var pop=document.getElementById('attachMenuPopover');if(pop&&pop.classList.contains('open')){closeAttachMenu();}else{openAttachMenu();}});
}
document.addEventListener('click',function(e){
  var pop=document.getElementById('attachMenuPopover');if(!pop||!pop.classList.contains('open'))return;
  var sub=document.getElementById('rolesSubmenuPopover');
  if(!pop.contains(e.target)&&!(sub&&sub.contains(e.target))&&e.target!==attachImgBtn){closeAttachMenu();}
});
(function(){
  var pop=document.getElementById('attachMenuPopover');if(!pop)return;
  pop.querySelectorAll('.attach-menu-item').forEach(function(btn){
    btn.addEventListener('click',function(e){e.preventDefault();e.stopPropagation();
      var mode=btn.dataset.mode,action=btn.dataset.action;
      if(mode){currentMode=mode;var sub=document.getElementById('rolesSubmenuPopover');if(sub)sub.querySelectorAll('.attach-menu-item.mode-btn').forEach(function(b){b.classList.remove('active');b.setAttribute('aria-pressed','false');});btn.classList.add('active');btn.setAttribute('aria-pressed','true');if(typeof updateChatModeUI==='function')updateChatModeUI();closeAttachMenu();return;}
      if(action==='roles'){var sub=document.getElementById('rolesSubmenuPopover');if(sub&&sub.classList.contains('open'))closeRolesSubmenu();else openRolesSubmenu();return;}
      if(action==='copy'){closeAttachMenu();if(typeof copyConversationToClipboard==='function')copyConversationToClipboard();return;}
      if(action==='export'){closeAttachMenu();if(typeof exportConversationAsMarkdown==='function')exportConversationAsMarkdown();return;}
      if(action==='refresh'){closeAttachMenu();location.reload();return;}
      if(action==='photos'){closeAttachMenu();if(imgFile)imgFile.click();return;}
      if(action==='files'){closeAttachMenu();if(fileInput)fileInput.click();return;}
      if(action==='screenshot'){closeAttachMenu();var el=document.getElementById('chatArea');if(!el)return;function capture(){if(typeof html2canvas!=='function'){alert('Screenshot not available. Try refreshing.');return;}html2canvas(el,{useCORS:true,logging:false,scale:window.devicePixelRatio||1}).then(function(canvas){canvas.toBlob(function(blob){if(!blob)return;var a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='locus-chat-'+Date.now()+'.png';a.click();URL.revokeObjectURL(a.href);},'image/png');}).catch(function(){alert('Could not capture chat.');});}if(typeof html2canvas!=='function'){var s=document.createElement('script');s.src='https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js';s.onload=capture;s.onerror=function(){alert('Could not load screenshot tool.');};document.head.appendChild(s);}else{capture();}return;}
    });
  });
  if(typeof updateChatModeUI==='function')updateChatModeUI();
})();
(function(){
  var wrap=document.getElementById('threadSwitcher');if(!wrap)return;
  var prev=wrap.querySelector('.thread-switcher-prev');var next=wrap.querySelector('.thread-switcher-next');
  if(prev)prev.addEventListener('click',function(){if(currentBranchIndex<=0||!currentId)return;currentBranchIndex--;loadConvo(currentId,currentSrc);});
  if(next)next.addEventListener('click',function(){if(currentBranchIndex>=branchCount-1||!currentId)return;currentBranchIndex++;loadConvo(currentId,currentSrc);});
})();
function getFileBubbleType(fileName,mime){
  if(!fileName)return 'file';
  var ext=(fileName.split('.').pop()||'').toLowerCase();
  if(mime==='application/pdf'||ext==='pdf')return 'pdf';
  if(ext==='md'||ext==='markdown')return 'markdown';
  if(/^(mp3|wav|m4a|ogg|webm|aac|flac|opus|weba)$/.test(ext)||(mime&&mime.indexOf('audio/')===0))return 'audio';
  if(/^(png|jpg|jpeg|gif|webp|svg|bmp|ico)$/.test(ext)||(mime&&mime.indexOf('image/')===0))return 'image';
  return 'text';
}
function updateFileBubbles(){
  var container=document.getElementById('fileBubbles');if(!container)return;
  var prevCount=container.children.length;
  container.textContent='';
  pendingAttachments.forEach(function(a,idx){
    var b=document.createElement('div');
    var rm=document.createElement('button');rm.type='button';rm.className='file-bubble-remove';rm.setAttribute('aria-label','Remove');rm.textContent='\u00D7';
    rm.onclick=function(){pendingAttachments.splice(idx,1);if(msgInput)msgInput.placeholder=pendingAttachments.length?'Add a message or send.':'Message...';updateFileBubbles();};
    if(a.type==='image'){
      b.className='file-bubble file-bubble--image';
      var src=(a.imageBase64||'').indexOf('base64,')>=0?a.imageBase64:'data:image/jpeg;base64,'+a.imageBase64;
      var img=document.createElement('img');img.className='file-bubble-thumb';img.src=src;img.alt='Photo';
      var lbl=document.createElement('span');lbl.className='file-bubble-label';lbl.textContent='Photo';
      b.appendChild(img);b.appendChild(lbl);b.appendChild(rm);
    }else{
      var type=getFileBubbleType(a.name,a.fileMime);
      b.className='file-bubble file-bubble--'+type;
      var icon=document.createElement('span');icon.className='file-bubble-icon';icon.textContent='\uD83D\uDCC4';icon.setAttribute('aria-hidden','true');
      var lbl=document.createElement('span');lbl.className='file-bubble-label';lbl.textContent=a.name;
      b.appendChild(icon);b.appendChild(lbl);b.appendChild(rm);
    }
    container.appendChild(b);
    if(idx>=prevCount){
      b.classList.add('file-bubble-enter');
      b.addEventListener('animationend',function once(){b.classList.remove('file-bubble-enter');b.removeEventListener('animationend',once);});
    }
  });
}
function showAttachPreview(){updateFileBubbles();}
function hideAttachPreview(){updateFileBubbles();}
if(imgFile){imgFile.addEventListener('change',function(){
  var f=this.files&&this.files[0];if(!f)return;
  this.value='';
  if(!canAddImage()){if(msgInput)msgInput.placeholder='Max one image. Remove it or send first.';return;}
  fileToBase64(f,function(b64){
    var existing=pendingAttachments.findIndex(function(a){return a.type==='image';});
    var entry={type:'image',name:'Photo',imageBase64:b64};
    if(existing>=0)pendingAttachments[existing]=entry;else pendingAttachments.push(entry);
    if(msgInput)msgInput.placeholder='Image attached. Add a message or send.';
    showAttachPreview();
  });
});}
if(fileInput){fileInput.addEventListener('change',function(){
  var f=this.files&&this.files[0];if(!f)return;
  this.value='';
  if(!canAddFile()){if(msgInput)msgInput.placeholder='Max '+MAX_FILE_ATTACHMENTS+' files. Remove some or send first.';return;}
  if(f.type==='application/pdf'){
    fileToBase64(f,function(b64){
      pendingAttachments.push({type:'file',name:f.name,fileBase64:b64,fileMime:'application/pdf'});
      if(msgInput)msgInput.placeholder='PDF attached. Add a message or send.';updateFileBubbles();
    });
    return;
  }
  if(TEXT_EXT.test(f.name)||f.type.indexOf('text/')===0||f.type==='application/json'||f.type==='application/xml'){
    var r=new FileReader();
    r.onload=function(){
      pendingAttachments.push({type:'file',name:f.name,fileText:r.result,fileMime:f.type||null});
      if(msgInput)msgInput.placeholder='File attached. Add a message or send.';updateFileBubbles();
    };
    r.readAsText(f,'UTF-8');
    return;
  }
  if(msgInput)msgInput.placeholder='Unsupported type. Use PDF or text (.txt, .md, .json, etc).';
});}
msgInput.addEventListener('paste',function(e){
  var items=e.clipboardData&&e.clipboardData.items;if(!items)return;
  for(var i=0;i<items.length;i++){var item=items[i];if(item.type.indexOf('image')!==-1){e.preventDefault();if(!canAddImage())return;var file=item.getAsFile();if(!file)return;fileToBase64(file,function(b64){var existing=pendingAttachments.findIndex(function(a){return a.type==='image';});var entry={type:'image',name:'Photo',imageBase64:b64};if(existing>=0)pendingAttachments[existing]=entry;else pendingAttachments.push(entry);if(msgInput)msgInput.placeholder='Image pasted. Add a message or send.';showAttachPreview();});break;}}
});
function isCurrentConvoInMobileList(){
  if(currentSrc!=='mobile'||!currentId)return false;
  return mobileConvos.some(function(c){return String(c.id)===String(currentId);});
}
async function doOneSend(content,sentId){
  sendInProgress=true;
  sendBtn.disabled=true;typing.classList.add('show');setHeaderMood('thinking');scrollBottom();
  try{
    var body={content:content,mode:currentMode||'bestie'};
    if(branchCount>1)body.branch_index=currentBranchIndex;
    var r=await fetch('/conversations/'+encodeURIComponent(sentId)+'/messages',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}));
    var data=null;try{data=await r.json();}catch(_){}
    typing.classList.remove('show');setHeaderMood(null);
    if(!r.ok){if(currentId===sentId)addBubble('assistant',(data&&data.detail)?String(data.detail):'Error '+r.status+' — try again.',true);if(r.status===404)fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r2){return r2.ok?r2.json():null;}).then(function(d){if(d&&Array.isArray(d.conversations)){mobileConvos=d.conversations;renderSidebar();}}).catch(function(){});}
    else{
      if(currentId===sentId){
        var dReplies=data&&data.replies&&Array.isArray(data.replies)?data.replies:null;
        if(dReplies&&dReplies.length>0){dReplies.forEach(function(part,i){var isLast=i===dReplies.length-1;var imgPath=isLast&&(data.generated_image_path||data.image_path)?(data.generated_image_path||data.image_path):null;addBubble('assistant',part.content||'',true,isLast?(data.quick_replies||null):null,false,{style:part.style||'final',imagePath:imgPath||undefined});});}else{var replyText=(data&&data.reply)!==undefined?data.reply:'(no reply)';var imgPath=data&&data.generated_image_path?data.generated_image_path:(data&&data.image_path?data.image_path:null);addBubble('assistant',replyText,true,data&&data.quick_replies,false,imgPath?{imagePath:imgPath}:undefined);}
        playDoneThinkingSound();
      }
      if(data&&data.title){var idx=mobileConvos.findIndex(function(c){return c.id===sentId});if(idx>=0){mobileConvos[idx]=Object.assign({},mobileConvos[idx],{title:data.title,updated_at:new Date().toISOString()});renderSidebar();}}
      if(currentId===sentId){var q=branchCount>1?'?branch='+currentBranchIndex:'';fetch('/conversations/'+encodeURIComponent(sentId)+q,mergeApiHeaders({})).then(function(r2){return r2.ok?r2.json():null;}).then(async function(c){if(!c||!c.messages||currentId!==sentId)return;currentMessages=c.messages;if(c.branch_count)branchCount=c.branch_count;currentFeedback={};try{var fr=await fetch('/conversations/'+encodeURIComponent(sentId)+'/feedback',mergeApiHeaders({}));if(fr.ok){var fd=await fr.json();if(fd&&fd.feedback&&typeof fd.feedback==='object'){for(var k in fd.feedback){var i=parseInt(k,10);if(!isNaN(i))currentFeedback[i]=fd.feedback[k];}}}}catch(_){}
if(currentId!==sentId)return;renderMessages(currentMessages,false,'mobile');updateCopyConvoButtonVisibility();scrollBottom();}).catch(function(){});}
    }
  }catch(e){typing.classList.remove('show');setHeaderMood(null);if(currentId===sentId)addBubble('assistant','Connection error: '+(e.message||e),true);}
  finally{sendInProgress=false;sendBtn.disabled=false;drainPendingQueue(sentId);}
}
function drainPendingQueue(sentId){
  if(!pendingDoubleTextQueue.length){msgInput.focus();return;}
  var sameId=pendingDoubleTextQueue.filter(function(x){return String(x.sentId)===String(sentId);});
  if(sameId.length===0){var next=pendingDoubleTextQueue.shift();doOneSend(next.content,next.sentId);return;}
  while(pendingDoubleTextQueue.length&&String(pendingDoubleTextQueue[0].sentId)===String(sentId)){pendingDoubleTextQueue.shift();}
  var batch=sameId.map(function(x){return x.content;});
  if(batch.length===1){doOneSend(batch[0],sentId);return;}
  doBatchSend(batch,sentId);
}
async function doBatchSend(contents,sentId){
  sendInProgress=true;sendBtn.disabled=true;typing.classList.add('show');setHeaderMood('thinking');scrollBottom();
  try {
    {
    var body={batch:contents,mode:currentMode||'bestie'};
    if(branchCount>1)body.branch_index=currentBranchIndex;
    var r=await fetch('/conversations/'+encodeURIComponent(sentId)+'/messages',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}));
    var data=null;try{data=await r.json();}catch(_){}
    typing.classList.remove('show');setHeaderMood(null);
    if(!r.ok){if(currentId===sentId)addBubble('assistant',(data&&data.detail)?String(data.detail):'Error '+r.status+' — try again.',true);}
    else if(currentId===sentId){
      var bReplies=data&&data.replies&&Array.isArray(data.replies)?data.replies:null;
      if(bReplies&&bReplies.length>0){bReplies.forEach(function(part,i){addBubble('assistant',part.content||'',true,i===bReplies.length-1?(data.quick_replies||null):null,false,{style:part.style||'final'});});}else{addBubble('assistant',(data&&data.reply)!==undefined?data.reply:'(no reply)',true,data&&data.quick_replies);}
      playDoneThinkingSound();
      var q=branchCount>1?'?branch='+currentBranchIndex:'';
      fetch('/conversations/'+encodeURIComponent(sentId)+q,mergeApiHeaders({})).then(function(r2){return r2.ok?r2.json():null;}).then(async function(c){if(!c||!c.messages||currentId!==sentId)return;currentMessages=c.messages;if(c.branch_count)branchCount=c.branch_count;currentFeedback={};try{var fr=await fetch('/conversations/'+encodeURIComponent(sentId)+'/feedback',mergeApiHeaders({}));if(fr.ok){var fd=await fr.json();if(fd&&fd.feedback&&typeof fd.feedback==='object'){for(var k in fd.feedback){var i=parseInt(k,10);if(!isNaN(i))currentFeedback[i]=fd.feedback[k];}}}}catch(_){}
if(currentId!==sentId)return;renderMessages(currentMessages,false,'mobile');updateCopyConvoButtonVisibility();scrollBottom();}).catch(function(){});}
    }
    if(data&&data.title){var idx=mobileConvos.findIndex(function(c){return c.id===sentId});if(idx>=0){mobileConvos[idx]=Object.assign({},mobileConvos[idx],{title:data.title,updated_at:new Date().toISOString()});renderSidebar();}}
  } catch(e){typing.classList.remove('show');setHeaderMood(null);if(currentId===sentId)addBubble('assistant','Connection error: '+(e.message||e),true);}
  finally { sendInProgress=false;sendBtn.disabled=false;if(pendingDoubleTextQueue.length){var next=pendingDoubleTextQueue.shift();doOneSend(next.content,next.sentId);}else msgInput.focus(); }
}
async function send(){
  if(currentRO)return;
  if(isGroupView){
    sendGroupMessage();return;
  }
  if(currentSrc!=='mobile')return;
  if(!currentId){msgInput.focus();return;}
  if(!isCurrentConvoInMobileList()){
    addBubble('assistant','This chat is no longer in your list (e.g. deleted or wrong account). Refreshing…',true);
    loadAll().catch(function(){});
    return;
  }
  if(sendInProgress){
    var text=msgInput.value.trim();
    if(!text)return;
    pendingDoubleTextQueue.push({content:text,sentId:currentId});
    addBubble('user',text,true);msgInput.value='';updateContextIndicator();clearDraft(currentId,'mobile');autoResize();
    return;
  }
  var text=msgInput.value.trim();
  try{
    var fp=sessionStorage.getItem('locus_pending_file_path');
    var fc=sessionStorage.getItem('locus_pending_file_content');
    if(fp&&fc!==null){ var cap=25000; var excerpt=fc.length>cap?fc.slice(0,cap)+'\n...[truncated]':fc; text=(text?text+'\n\n':'')+'[File: '+fp+']\n\n'+excerpt; sessionStorage.removeItem('locus_pending_file_path'); sessionStorage.removeItem('locus_pending_file_content'); var b=document.getElementById('pendingFileBanner'); if(b)b.remove(); }
  }catch(e){}
  var hasAttach=pendingAttachments.length>0;
  if(!text&&!hasAttach)return;if(!currentId)return;
  var imgAtt=pendingImage();var fileList=pendingFiles();
  var displayText=text||(imgAtt?'[Image]':(fileList.length?fileList.length===1?'[File: '+fileList[0].name+']':'['+fileList.length+' files]':'[File]'));
  if(displayText.length>120)displayText=displayText.slice(0,117)+'...';
  var imgDataUrl=imgAtt&&imgAtt.imageBase64?(imgAtt.imageBase64.indexOf('base64,')>=0?imgAtt.imageBase64:'data:image/jpeg;base64,'+imgAtt.imageBase64):null;
  var fileOpts=fileList.length?fileList.map(function(f){return {name:f.name};}):[];
  var sentId=currentId;addBubble('user',displayText,true,null,false,{imageDataUrl:imgDataUrl||null,files:fileOpts.length?fileOpts:null});
  msgInput.value='';updateContextIndicator();
  clearDraft(sentId,'mobile');
  var imgToSend=imgAtt?imgAtt.imageBase64:null;
  var textParts=[text||''];
  var filesToSend=[];
  pendingAttachments.forEach(function(a){
    if(a.type==='file'){
      if(a.fileText!=null){textParts.push('[Attached: '+a.name+']:\n'+a.fileText);}
      else if(a.fileBase64){filesToSend.push({file_base64:a.fileBase64,file_name:a.name,file_mime:a.fileMime||'application/pdf'});}
    }
  });
  var contentToSend=textParts.join('\n\n').trim()||'(no text)';
  pendingAttachments=[];
  hideAttachPreview();
  msgInput.placeholder='Message... (or attach image/file, paste image)';autoResize();
  sendInProgress=true;
  var useGeneratingUI=isImageRequest(contentToSend);
  if(useGeneratingUI&&generatingImage){generatingImage.classList.add('show');typing.classList.remove('show');}else{typing.classList.add('show');if(generatingImage)generatingImage.classList.remove('show');}
  sendBtn.disabled=true;if(typing.classList.contains('show'))setHeaderMood('thinking');else setHeaderMood(moodFromText(contentToSend));scrollBottom();
  try{
var body={content:contentToSend,image_base64:imgToSend||null,mode:currentMode||'bestie'};
if(filesToSend.length===1){body.file_base64=filesToSend[0].file_base64;body.file_name=filesToSend[0].file_name;body.file_mime=filesToSend[0].file_mime;}
else if(filesToSend.length>1){body.files=filesToSend;}
if(branchCount>1)body.branch_index=currentBranchIndex;
var r=await fetch('/conversations/'+encodeURIComponent(sentId)+'/messages',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}));
var data=null;try{data=await r.json();}catch(_){}
typing.classList.remove('show');if(generatingImage)generatingImage.classList.remove('show');setHeaderMood(null);
if(!r.ok){
  if(currentId===sentId){addBubble('assistant',(data&&data.detail)?String(data.detail):'Error '+r.status+' — try again.',true);if(r.status===404)fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r2){return r2.ok?r2.json():null;}).then(function(d){if(d&&Array.isArray(d.conversations)){mobileConvos=d.conversations;renderSidebar();}}).catch(function(){});}
}else{
  if(currentId===sentId){
    var replies=data&&data.replies&&Array.isArray(data.replies)?data.replies:null;
    if(replies&&replies.length>0){
      replies.forEach(function(part,i){
        var isLast=i===replies.length-1;
        var imgPath=isLast&&(data.generated_image_path||data.image_path)?(data.generated_image_path||data.image_path):null;
        addBubble('assistant',part.content||'',true,isLast?(data.quick_replies||null):null,false,{style:part.style||'final',imagePath:imgPath||undefined});
      });
    }else{
      var replyText=(data&&data.reply)!==undefined?data.reply:'(no reply)';
      var imgPath=data&&data.generated_image_path?data.generated_image_path:(data&&data.image_path?data.image_path:null);
      addBubble('assistant',replyText,true,data&&data.quick_replies,false,imgPath?{imagePath:imgPath}:undefined);
    }
    playDoneThinkingSound();
  }
  /* Update local conversation title/timestamp and currentMessages so Copy conversation stays accurate (incl. image descriptions) */
  if(data&&data.title){var idx=mobileConvos.findIndex(function(c){return c.id===sentId});if(idx>=0){mobileConvos[idx]=Object.assign({},mobileConvos[idx],{title:data.title,updated_at:new Date().toISOString()});renderSidebar();}}
  if(currentId===sentId){var q=branchCount>1?'?branch='+currentBranchIndex:'';fetch('/conversations/'+encodeURIComponent(sentId)+q,mergeApiHeaders({})).then(function(r){return r.ok?r.json():null;}).then(async function(c){if(!c||!c.messages||currentId!==sentId)return;currentMessages=c.messages;if(c.branch_count)branchCount=c.branch_count;currentFeedback={};try{var fr=await fetch('/conversations/'+encodeURIComponent(sentId)+'/feedback',mergeApiHeaders({}));if(fr.ok){var fd=await fr.json();if(fd&&fd.feedback&&typeof fd.feedback==='object'){for(var k in fd.feedback){var i=parseInt(k,10);if(!isNaN(i))currentFeedback[i]=fd.feedback[k];}}}}catch(_){}
if(currentId!==sentId)return;renderMessages(currentMessages,false,'mobile');updateCopyConvoButtonVisibility();scrollBottom();}).catch(function(){});}
}
  }catch(e){typing.classList.remove('show');if(generatingImage)generatingImage.classList.remove('show');setHeaderMood(null);if(currentId===sentId)addBubble('assistant','Connection error: '+(e.message||e),true)}
  finally{sendInProgress=false;sendBtn.disabled=false;drainPendingQueue(sentId);}
}

/* ── Fork (continue read-only chat with Claudia) ── */
async function forkConvoMobile(){
  if(!currentMessages||currentMessages.length===0)return;
  var btn=document.getElementById('forkBtn')||forkConvoBtn;
  if(btn){btn.disabled=true;}
  try{
  var title=(currentConvoTitle||'Chat')+' (continued)';
  var r=await fetch('/conversations/fork',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({title:title,messages:currentMessages})}));
  if(!r.ok)throw new Error('fork failed');
  var nc=await r.json();
  mobileConvos=[nc].concat(mobileConvos.filter(function(x){return x.id!==nc.id;}));
  currentId=nc.id;currentSrc='mobile';currentMessages=nc.messages||[];currentConvoTitle=nc.title||'New chat';
  renderSidebar();renderMessages(currentMessages,false,'mobile',{cascade:true});
  updateCopyConvoButtonVisibility();closeSidebar();msgInput.focus();
  }catch(e){if(btn){btn.disabled=false;}alert('Could not fork: '+(e.message||e));}
}
async function forkConvo(){
  var ep={continue:'/continue/conversations/',grok:'/grok/conversations/',cursor:'/cursor/conversations/'};
  var base=ep[currentSrc];if(!base)return;
  var btn=document.getElementById('forkBtn')||forkConvoBtn;
  if(btn){btn.textContent='Creating...';btn.disabled=true;}
  try{
var branchQ=(currentSrc==='grok'&&branchCount>1)?'?branch=0':'';
var r=await fetch(base+encodeURIComponent(currentId)+branchQ);
if(!r.ok)throw new Error('fetch failed');
var data=await r.json();
var msgs=data.messages||[];
var title=(data.title||'Chat')+' (continued)';
var body={title:title};
if(data.branches&&Array.isArray(data.branches)&&data.branches.length>1){body.branches=data.branches;}else{body.messages=msgs;}
var r2=await fetch('/conversations/fork',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}));
if(!r2.ok)throw new Error('fork failed');
var nc=await r2.json();
mobileConvos=[nc].concat(mobileConvos.filter(function(x){return x.id!==nc.id;}));
currentId=nc.id;currentSrc='mobile';currentMessages=nc.messages||[];currentConvoTitle=nc.title||'New chat';
currentBranchIndex=nc.branch_index!==undefined?nc.branch_index:0;branchCount=nc.branch_count||1;
renderSidebar();renderMessages(currentMessages,false,'mobile',{cascade:true});updateThreadSwitcher();
closeSidebar();msgInput.focus();
  }catch(e){
if(btn){if(btn.id==='forkBtn')btn.textContent='\u25B6\uFE0F Continue with Locus';else if(forkConvoBtn&&btn===forkConvoBtn)btn.innerHTML=ICON_FORK;btn.disabled=false;}
alert('Could not fork conversation: '+(e.message||e));
  }
}
if(forkConvoBtn)forkConvoBtn.addEventListener('click',function(){if(!confirm('Fork to new chat?'))return;setTimeout(function(){if(currentSrc==='mobile')forkConvoMobile();else forkConvo();},0);});
/* ── New chat ── */
async function newChat(){
  var r=await fetch('/conversations',mergeApiHeaders({method:'POST'}));if(!r.ok)return;
  var c=await r.json();
  mobileConvos=[c].concat(mobileConvos.filter(function(x){return x.id!==c.id}));
  currentId=c.id;currentSrc='mobile';currentMessages=[];currentConvoTitle=c.title||'New chat';
  currentBranchIndex=0;branchCount=1;
  renderSidebar();renderMessages([],false,'mobile');
  updateThreadSwitcher();updateCopyConvoButtonVisibility();closeSidebar();msgInput.focus();
}
sbNew.addEventListener('click',newChat);
var headerNewBtn=document.getElementById('headerNewBtn');
if(headerNewBtn)headerNewBtn.addEventListener('click',newChat);
if(hdrCopyExportBtn){
  hdrCopyExportBtn.addEventListener('click',function(e){e.stopPropagation();var open=!hdrCopyExportDropdown.classList.contains('open');hdrCopyExportDropdown.classList.toggle('open',open);hdrCopyExportDropdown.setAttribute('aria-hidden',open?'false':'true');hdrCopyExportBtn.setAttribute('aria-expanded',open?'true':'false');});
}
if(copyConvoAction)copyConvoAction.addEventListener('click',function(){copyConversationToClipboard();closeCopyExportDropdown();});
if(exportConvoAction)exportConvoAction.addEventListener('click',function(){exportConversationAsMarkdown();closeCopyExportDropdown();});
var refreshAppAction=document.getElementById('refreshAppAction');
if(refreshAppAction)refreshAppAction.addEventListener('click',function(){closeCopyExportDropdown();location.reload();});
var refreshAppBtn=document.getElementById('refreshAppBtn');
if(refreshAppBtn)refreshAppBtn.addEventListener('click',function(){location.reload();});
document.addEventListener('click',function(){closeCopyExportDropdown();});
/* ── Input auto-resize & keyboard ── */
function autoResize(){msgInput.style.height='auto';msgInput.style.height=Math.min(msgInput.scrollHeight,130)+'px'}
function updateContextIndicator(){
  var el=document.getElementById('contextIndicator'),arc=document.getElementById('contextIndicatorArc');
  if(!el||!arc||!msgInput)return;
  var len=(msgInput.value||'').length;
  var cap=1000;
  var pct=cap<=0?0:Math.min(1,len/cap);
  arc.setAttribute('stroke-dashoffset',String(Math.round(88*(1-pct))));
  el.classList.remove('medium','high');
  if(pct>=.5)el.classList.add('medium');
  if(pct>=.85)el.classList.add('high');
}
msgInput.addEventListener('input',function(){ autoResize(); saveDraftDebounced(); hideDraftRestoredHint(); updateContextIndicator(); });
msgInput.addEventListener('keydown',function(e){
  if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){
e.preventDefault();send();
  }
});
sendBtn.addEventListener('click',send);
  msgInput.addEventListener('focus',function(){setTimeout(scrollBottom,350)});

/* ── Tab bar (Chat / Room) ── */
var currentTab='chat';
var bedroomTick=null;
var bedroomWalkEnd=null;
var WAYPOINTS=[{id:'bed',x:52,y:72},{id:'tv',x:48,y:40},{id:'plants',x:20,y:52},{id:'window',x:76,y:48},{id:'center',x:44,y:58}];
var ACTIVITIES={bed:'\uD83D\uDCA4',tv:'\uD83D\uDCFA',plants:'\uD83C\uDF3F',window:'\u2615',center:''};
function setSpritePosition(xPct,yPct){
  if(!locusSprite)return;
  locusSprite.style.left=xPct+'%';locusSprite.style.top=yPct+'%';
}
function showActivity(emoji){
  if(!activityBubble)return;
  activityBubble.textContent=emoji||'';
  activityBubble.className='show'+(emoji==='\uD83D\uDCA4'?' zzz':'');
  activityBubble.setAttribute('aria-label',emoji?'Activity: '+emoji:'');
}
function hideActivity(){
  if(!activityBubble)return;
  activityBubble.className='';activityBubble.textContent='';
}
function pickNext(){
  var idx=Math.floor(Math.random()*WAYPOINTS.length);
  return WAYPOINTS[idx];
}
function runBedroomLoop(){
  if(currentTab!=='room'||!locusSprite||!roomPanel)return;
  var dest=pickNext();
  locusSprite.classList.remove('sleep');locusSprite.classList.add('walk');
  setSpritePosition(dest.x,dest.y);
  if(bedroomWalkEnd)clearTimeout(bedroomWalkEnd);
  bedroomWalkEnd=setTimeout(function(){
    bedroomWalkEnd=null;
    locusSprite.classList.remove('walk');
    var emoji=ACTIVITIES[dest.id];
    if(emoji){showActivity(emoji);if(dest.id==='bed')locusSprite.classList.add('sleep');}
    var duration=4000+Math.random()*4000;
    setTimeout(function(){
      hideActivity();locusSprite.classList.remove('sleep');
      bedroomTick=setTimeout(runBedroomLoop,800+Math.random()*1200);
    },duration);
  },2600);
}
function startBedroom(){runBedroomLoop();}
function stopBedroom(){
  if(bedroomTick){clearTimeout(bedroomTick);bedroomTick=null;}
  if(bedroomWalkEnd){clearTimeout(bedroomWalkEnd);bedroomWalkEnd=null;}
  hideActivity();
  if(locusSprite){locusSprite.classList.remove('walk','sleep');}
}
function switchToTab(tab){
  currentTab=tab;
  if(!appEl)return;
  appEl.classList.toggle('show-room',tab==='room');
  if(roomPanel)roomPanel.setAttribute('aria-hidden',tab!=='room');
  var tabs=tabBar?tabBar.querySelectorAll('.tab'):[];
  tabs.forEach(function(t){var isActive=t.getAttribute('data-tab')===tab;t.classList.toggle('active',isActive);t.setAttribute('aria-selected',isActive);});
  if(tab==='room'){startBedroom();}else{stopBedroom();}
  isGroupView=(tab==='social');
  if(isGroupView){
    if(hdrName)hdrName.textContent='Group';
    if(hdrSub)hdrSub.textContent='Ruby, Lynn, Locus, Raven';
    loadGroupChat();
  }else{
    if(hdrName)hdrName.textContent=currentConvoTitle||'Chat';
    if(hdrSub)hdrSub.textContent=currentSrc==='mobile'?'Locus':(currentSrc||'');
    if(currentId)loadConvo(currentId,currentSrc);
    else{clearChat();showEmpty();}
  }
  updateCopyConvoButtonVisibility();
}
async function loadGroupChat(){
  try{
    var r=await fetch('/api/group_chat',mergeApiHeaders({cache:'no-store'}));
    if(!r.ok){clearChat();showEmpty(true);currentMessages=[];return;}
    var d=await r.json();
    currentMessages=d.messages||[];
    if(!currentMessages.length){clearChat();showEmpty(true);}
    else{renderMessages(currentMessages,false,'mobile',{group:true});scrollBottom();}
  }catch(e){clearChat();showEmpty(true);currentMessages=[];}
  updateCopyConvoButtonVisibility();
}
if(tabBar){
  tabBar.querySelectorAll('.tab').forEach(function(btn){
    btn.addEventListener('click',function(){var t=btn.getAttribute('data-tab');if(t)switchToTab(t);});
  });
}

/* ── Avatar + profile + init (no auth) ── */
function fetchAvatarMe(){
  fetch('/api/avatar/me',mergeApiHeaders({}))
    .then(function(r){return r.ok?r.json():null;})
    .then(function(d){
      if(d&&d.avatarUrl){
        currentUserAvatarUrl=d.avatarUrl;
        try{ if(typeof renderMessages==='function' && currentMessages && currentMessages.length){ renderMessages(); } }catch(_){}
      }
    })
    .catch(function(){});
}
function loadAvatarPicker(){
  if(!avatarPicker)return;
  fetch('/api/avatar/characters').then(function(r){return r.ok?r.json():null;}).then(function(d){avatarCharacters=(d&&d.characters)||[];}).then(function(){
    return fetch('/api/avatar/me',mergeApiHeaders({})).then(function(r){return r.ok?r.json():null;});
  }).then(function(me){
    var selectedId=(me&&me.characterId)||'';
    avatarPicker.innerHTML='';
    var fallbackAvatarUrl='/user_avatar.svg';
    avatarCharacters.forEach(function(c){
      var btn=document.createElement('button');btn.type='button';btn.className='av-opt'+(c.id===selectedId?' selected':'');btn.dataset.characterId=c.id;btn.setAttribute('aria-label',c.name);
      var img=document.createElement('img');img.src=c.avatarUrl;img.alt='';img.loading='lazy';
      img.onerror=function(){if(this.src&&this.src!==fallbackAvatarUrl){this.src=fallbackAvatarUrl;} };
      btn.appendChild(img);
      btn.onclick=function(){
        fetch('/api/avatar/me',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({character_id:c.id})}))
          .then(function(r){return r.ok?r.json():null;})
          .then(function(d){
            if(d&&d.avatarUrl){
              currentUserAvatarUrl=d.avatarUrl;
              try{ if(typeof renderMessages==='function'){ renderMessages(); } }catch(_){}
            }
            loadAvatarPicker();
          });
      };
      avatarPicker.appendChild(btn);
    });
  }).catch(function(){});
}
if(avatarCustomBtn&&avatarCustomUrl){
  avatarCustomBtn.addEventListener('click',function(){
    var url=(avatarCustomUrl.value||'').trim();if(!url)return;
    fetch('/api/avatar/me',mergeApiHeaders({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({custom_url:url})}))
      .then(function(r){return r.ok?r.json():null;})
      .then(function(d){
        if(d&&d.avatarUrl){
          currentUserAvatarUrl=d.avatarUrl;
          try{ if(typeof renderMessages==='function'){ renderMessages(); } }catch(_){}
        }
        loadAvatarPicker();
      });
  });
}
function showUserProfileSection(){
  if(!userProfileWrap)return;
  userProfileWrap.style.display=getCurrentUser()?'block':'none';
  if(getCurrentUser())loadUserProfile();
}
function loadUserProfile(){
  if(!userProfilePronouns||!userProfileAbout)return;
  fetch('/api/user/profile',mergeApiHeaders({})).then(function(r){return r.ok?r.json():null;}).then(function(d){
    if(d){userProfilePronouns.value=d.pronouns||'';userProfileAbout.value=d.about_me||'';}
  }).catch(function(){});
}
function saveUserProfile(){
  if(!userProfileSave||!userProfilePronouns||!userProfileAbout)return;
  userProfileSave.disabled=true;
  fetch('/api/user/profile',mergeApiHeaders({method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({pronouns:userProfilePronouns.value.trim(),about_me:userProfileAbout.value.trim()})})).then(function(r){return r.ok?r.json():null;}).then(function(){
    userProfileSave.disabled=false;
    var t=userProfileSave.textContent;userProfileSave.textContent='Saved';setTimeout(function(){userProfileSave.textContent=t;},1200);
  }).catch(function(){userProfileSave.disabled=false;});
}
/* ── Init ── */
showEmpty();
if(userSelect){
  try{ var u=getCurrentUser(); userSelect.value=u; setLocusUserCookie(u); }catch(e){}
  fetchAvatarMe();
  loadAvatarPicker();
  showUserProfileSection();
  if(userProfileSave)userProfileSave.addEventListener('click',saveUserProfile);
  userSelect.addEventListener('input',function(){
    var next=(userSelect.value||'').trim().toLowerCase();
    try{ localStorage.setItem('locus_user',next); }catch(e){}
    setLocusUserCookie(next);
    fetchAvatarMe();
    loadAvatarPicker();
    showUserProfileSection();
  });
  userSelect.addEventListener('blur',function(){
    loadAll().catch(function(){});
  });
} else {
  fetchAvatarMe();
  loadAvatarPicker();
}
loadAll().catch(function(e){
  clearChat();
  var err=document.createElement('div');err.className='empty-state';
  err.innerHTML='<div class="em-icon">&#9888;</div><div class="em-title">Cannot connect</div>'
  +'<div class="em-sub">Is Locus running?<br>Run: python scripts/locus_api.py</div>'
  +'<button type="button" class="sb-new" id="retryConnectBtn" style="margin-top:12px">Tap to retry</button>';
  chatArea.insertBefore(err,typing);
  var retryBtn=document.getElementById('retryConnectBtn');
  if(retryBtn){retryBtn.addEventListener('click',function(){ var p=err.parentNode; if(p){ p.removeChild(err); showEmpty(); loadAll().catch(function(){ p.insertBefore(err,typing); }); }});}
});
document.addEventListener('visibilitychange',function(){
  if(document.visibilityState!=='visible')return;
  fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r){return r.json();}).then(function(data){
    if(data&&data.conversations){mobileConvos=data.conversations;renderSidebar();}
  }).catch(function(){});
});
(function(){
  try{
    var path=sessionStorage.getItem('locus_pending_file_path');
    if(!path)return;
    var inputArea=document.getElementById('inputArea');
    if(!inputArea)return;
    var banner=document.createElement('div');
    banner.id='pendingFileBanner';
    banner.style.cssText='padding:8px 16px;background:rgba(255,122,217,.15);border-bottom:1px solid rgba(255,122,217,.3);font-size:13px;color:#f0eaff;display:flex;align-items:center;gap:10px;flex-wrap:wrap';
    var span=document.createElement('span');
    span.textContent='Including file: '+path+' — your next message will include it.';
    var btn=document.createElement('button');
    btn.type='button';btn.textContent='Clear';btn.style.cssText='margin-left:auto;padding:4px 10px;border-radius:8px;border:1px solid #ff7ad9;background:transparent;color:#ff7ad9;cursor:pointer;font-size:12px';
    btn.onclick=function(){sessionStorage.removeItem('locus_pending_file_path');sessionStorage.removeItem('locus_pending_file_content');banner.remove();};
    banner.appendChild(span);banner.appendChild(btn);
    inputArea.parentNode.insertBefore(banner,inputArea);
  }catch(e){}
})();
}catch(e){ showInitErr('Init error: '+e.message); }
})();
