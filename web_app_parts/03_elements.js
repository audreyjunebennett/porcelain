/* ── Elements ── */
var sidebar=document.getElementById('sidebar'),sideOverlay=document.getElementById('sideOverlay');
var sbList=document.getElementById('sbList'),sbSearch=document.getElementById('sbSearch'),sbNew=document.getElementById('sbNew');
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
var appEl=document.getElementById('app');
var tabBar=document.getElementById('tabBar');
var roomPanel=document.getElementById('roomPanel');
var claudiaSprite=document.getElementById('claudiaSprite');
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
