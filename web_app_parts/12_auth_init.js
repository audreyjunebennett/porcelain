/* ── Avatar picker ── */
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
/* ── Init: load conversations (no auth needed) ── */
showEmpty();
fetchAvatarMe();
loadAvatarPicker();
fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r){return r.json();}).then(function(data){
  if(data&&data.conversations){mobileConvos=data.conversations;renderSidebar();}
}).catch(function(){});
document.addEventListener('visibilitychange',function(){
  if(document.visibilityState!=='visible')return;
  fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r){return r.json();}).then(function(data){
    if(data&&data.conversations){mobileConvos=data.conversations;renderSidebar();}
  }).catch(function(){});
});
try{
  loadAll().catch(function(e){
    clearChat();
    var err=document.createElement('div');err.className='empty-state';
    err.innerHTML='<div class="em-icon">&#9888;</div><div class="em-title">Cannot connect</div>'
  +'<div class="em-sub">Is the server running?<br>Run: python pwa/server.py</div>'
  +'<button type="button" class="sb-new" id="retryConnectBtn" style="margin-top:12px">Tap to retry</button>';
    chatArea.insertBefore(err,typing);
    var retryBtn=document.getElementById('retryConnectBtn');
    if(retryBtn){retryBtn.addEventListener('click',function(){ var p=err.parentNode; if(p){ p.removeChild(err); showEmpty(); loadAll().catch(function(){ p.insertBefore(err,typing); }); }});}
  });
  (function(){
    try{
      var path=sessionStorage.getItem('claudia_pending_file_path');
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
      btn.onclick=function(){sessionStorage.removeItem('claudia_pending_file_path');sessionStorage.removeItem('claudia_pending_file_content');banner.remove();};
      banner.appendChild(span);banner.appendChild(btn);
      inputArea.parentNode.insertBefore(banner,inputArea);
    }catch(e){}
  })();
}catch(e){ showInitErr('Init error: '+e.message); }
