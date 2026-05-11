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
      if(action==='screenshot'){closeAttachMenu();var el=document.getElementById('chatArea');if(!el)return;function capture(){if(typeof html2canvas!=='function'){alert('Screenshot not available. Try refreshing.');return;}html2canvas(el,{useCORS:true,logging:false,scale:window.devicePixelRatio||1}).then(function(canvas){canvas.toBlob(function(blob){if(!blob)return;var a=document.createElement('a');a.href=URL.createObjectURL(blob);a.download='claudia-chat-'+Date.now()+'.png';a.click();URL.revokeObjectURL(a.href);},'image/png');}).catch(function(){alert('Could not capture chat.');});}if(typeof html2canvas!=='function'){var s=document.createElement('script');s.src='https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js';s.onload=capture;s.onerror=function(){alert('Could not load screenshot tool.');};document.head.appendChild(s);}else{capture();}return;}
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
    rm.onclick=function(){pendingAttachments.splice(idx,1);if(msgInput)msgInput.placeholder=pendingAttachments.length?'Add a message or send.':'Message Claudia...';updateFileBubbles();};
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
    var fp=sessionStorage.getItem('claudia_pending_file_path');
    var fc=sessionStorage.getItem('claudia_pending_file_content');
    if(fp&&fc!==null){ var cap=25000; var excerpt=fc.length>cap?fc.slice(0,cap)+'\n...[truncated]':fc; text=(text?text+'\n\n':'')+'[File: '+fp+']\n\n'+excerpt; sessionStorage.removeItem('claudia_pending_file_path'); sessionStorage.removeItem('claudia_pending_file_content'); var b=document.getElementById('pendingFileBanner'); if(b)b.remove(); }
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
  msgInput.placeholder='Message Claudia... (or attach image/file, paste image)';autoResize();
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
