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
    +'<div class="em-sub">Say anything &#8212; Claudia knows your memories and can search the web.</div>'
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
  else{av.className='msg-av';var ai=document.createElement('img');ai.src='/claudia_avatar.svg';ai.alt='';ai.style.cssText='width:40px;height:40px;max-width:40px;max-height:40px;object-fit:contain;display:block;flex-shrink:0';ai.onerror=function(){this.src='/chat_icon.png';this.onerror=function(){this.src='/icon.svg';};};av.appendChild(ai);}
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
      var sender=m.sender||(role==='assistant'?'claudia':'ruby');
      var senderLabel=sender==='claudia'?'Claudia':'You';
      var wrap=document.createElement('div');wrap.className='msg-wrap '+(role==='user'?'user':'assistant')+(isGroup?' group-msg':'')+(role==='assistant'&&m.style==='thinking'?' thinking-msg':'');
      if(isGroup){var senderRow=document.createElement('div');senderRow.className='msg-row msg-sender-row';senderRow.innerHTML='<span class="msg-sender-label">'+escapeHtml(senderLabel)+'</span>';wrap.appendChild(senderRow);}
      var row=document.createElement('div');row.className='msg-row';
      var av=document.createElement('div');
      if(role==='user'){av.className='msg-av user-av';var ui=document.createElement('img');ui.src=currentUserAvatarUrl;ui.alt='';ui.style.cssText='width:64px;height:64px;max-width:64px;max-height:64px;object-fit:contain;display:block;flex-shrink:0';ui.onerror=function(){av.textContent='\u273F';};av.appendChild(ui);}
      else{av.className='msg-av';var ai=document.createElement('img');ai.src='/claudia_avatar.svg';ai.alt='';ai.style.cssText='width:40px;height:40px;max-width:40px;max-height:40px;object-fit:contain;display:block;flex-shrink:0';ai.onerror=function(){this.src='/chat_icon.png';this.onerror=function(){this.src='/icon.svg';};};av.appendChild(ai);}
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
  if(convo){hdrName.textContent=convo.title||'Claudia \u2665';hdrSub.textContent=srcLabel(convo.source)+' chat'}
  else{hdrName.textContent='Claudia \u2665';hdrSub.textContent='Claudia Core \u00B7 same workspace as your PC'}
  if(ro){
roHint.style.display='flex';
var srcName={continue:'VS Code',grok:'Grok',cursor:'Cursor'}[src]||src;
roHint.innerHTML='<span>'+srcName+' chat \u2014 read only</span>'
  +'<button id="forkBtn">\u25B6\uFE0F Continue with Claudia</button>';
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
