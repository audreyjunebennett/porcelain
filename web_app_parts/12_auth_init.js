/* ── Auth: sign in / sign out (session cookie); sync "Chat as" when signed in ── */
var signInBtn=document.getElementById('signInBtn'),signOutBtn=document.getElementById('signOutBtn'),signedInAs=document.getElementById('signedInAs');
var loginOverlay=document.getElementById('loginOverlay'),loginForm=document.getElementById('loginForm'),loginUser=document.getElementById('loginUser'),loginUsername=document.getElementById('loginUsername'),loginPassword=document.getElementById('loginPassword'),loginCancel=document.getElementById('loginCancel'),loginSubmit=document.getElementById('loginSubmit'),loginError=document.getElementById('loginError');

function setSignedInState(signedIn,user,displayName){
  if(signInBtn){signInBtn.style.display=signedIn?'none':'block';}
  if(signedInAs){signedInAs.style.display=signedIn?'inline':'none';signedInAs.textContent=signedIn?'Signed in as '+(displayName||user||''):'';}
  if(signOutBtn){signOutBtn.style.display=signedIn?'block':'none';}
  if(!signedIn){continueConvos=[];grokConvos=[];cursorConvos=[];}
  if(signedIn&&user){try{localStorage.setItem('claudia_user',user);}catch(e){}setClaudiaUserCookie(user);if(userSelect){userSelect.value=user;}}
}
function hideLoginOverlay(){if(loginOverlay){loginOverlay.classList.remove('show');loginOverlay.setAttribute('aria-hidden','true');loginOverlay.setAttribute('inert','');}if(loginError){loginError.style.display='none';loginError.textContent='';}}
function showLoginOverlay(){if(loginOverlay){loginOverlay.classList.add('show');loginOverlay.removeAttribute('aria-hidden');loginOverlay.removeAttribute('inert');}}
var protectedUsers=[],signedInUser=null,lastSelectedUser=getCurrentUser();
function applyLockLabelsToDropdown(){if(!userSelect||!userSelect.options)return;var i,o;for(i=0;i<userSelect.options.length;i++){o=userSelect.options[i];o.textContent=userDisplayNames[o.value]||o.value;(protectedUsers.indexOf(o.value)>=0)&&(o.textContent+=' \uD83D\uDD12');}}
function showAccountLocked(){var who=userSelect?userSelect.value:'';if(loginUser){loginUser.value=who;}if(loginError){loginError.textContent='Sign in as '+(userDisplayNames[who]||who)+' to access her chats.';loginError.style.display='block';}showLoginOverlay();}
/* ── Init ── */
showEmpty(); // visible immediately while loadAll fetches
fetch('/api/auth/status',{credentials:'include',cache:'no-store'}).then(function(r){return r.ok?r.json():{};}).catch(function(){return {};}).then(function(d){
  protectedUsers=(d&&d.protectedUsers)||[];signedInUser=d&&d.signedInUser||null;
  if(signedInUser){setSignedInState(true,signedInUser,userDisplayNames[signedInUser]);if(userSelect){userSelect.value=signedInUser;}lastSelectedUser=signedInUser;fetchAvatarMe();loadAvatarPicker();}else{setSignedInState(false);}
  applyLockLabelsToDropdown();
  initAfterToken();
  function initAfterToken(){
  if(signInBtn){signInBtn.addEventListener('click',showLoginOverlay);}
  if(signOutBtn){signOutBtn.addEventListener('click',function(){fetch('/api/auth/logout',{method:'POST',credentials:'include'}).then(function(){location.reload();}).catch(function(){location.reload();});});}
  if(loginOverlay){
    var backdrop=loginOverlay.querySelector('.login-backdrop');
    if(backdrop){backdrop.addEventListener('click',hideLoginOverlay);}
  }
  if(loginCancel){loginCancel.addEventListener('click',hideLoginOverlay);}
  if(loginForm){
    if(loginUser&&loginUsername){loginUsername.value=loginUser.value||'';loginUser.addEventListener('change',function(){loginUsername.value=loginUser.value||'';});}
    loginForm.addEventListener('submit',function(e){e.preventDefault();if(!loginUser||!loginPassword)return;var u=(loginUser.value||'').trim(),p=loginPassword.value;if(!u){if(loginError){loginError.textContent='Pick who you are.';loginError.style.display='block';}return;}
    loginSubmit.disabled=true;loginError.style.display='none';loginError.textContent='';
    fetch('/api/auth/login',mergeApiHeaders({method:'POST',credentials:'include',headers:{'Content-Type':'application/json'},body:JSON.stringify({user:u,password:p})})).then(function(r){if(r.ok){hideLoginOverlay();location.reload();return;}return r.json().then(function(d){throw new Error(d.detail||'Login failed');});}).catch(function(err){if(loginError){loginError.textContent=err.message||'Login failed';loginError.style.display='block';}loginSubmit.disabled=false;});});
  }
  loadAll().catch(function(e){
    clearChat();
    var err=document.createElement('div');err.className='empty-state';
    err.innerHTML='<div class="em-icon">&#9888;</div><div class="em-title">Cannot connect</div>'
  +'<div class="em-sub">Is the server running?<br>Run: python Scripts/mobile_orchestrator_api.py</div>'
  +'<button type="button" class="sb-new" id="retryConnectBtn" style="margin-top:12px">Tap to retry</button>';
    chatArea.insertBefore(err,typing);
    var retryBtn=document.getElementById('retryConnectBtn');
    if(retryBtn){retryBtn.addEventListener('click',function(){ var p=err.parentNode; if(p){ p.removeChild(err); showEmpty(); loadAll().catch(function(){ p.insertBefore(err,typing); }); }});}
  });
  }
}).catch(function(){protectedUsers=[];signedInUser=null;setSignedInState(false);applyLockLabelsToDropdown();});
document.addEventListener('visibilitychange',function(){
  if(document.visibilityState!=='visible')return;
  fetch('/conversations?_='+Date.now(),mergeApiHeaders({cache:'no-store'})).then(function(r){return r.json();}).then(function(data){
    if(data&&data.conversations){mobileConvos=data.conversations;renderSidebar();}
  }).catch(function(){});
});
function fetchAvatarMe(){
  fetch('/api/avatar/me',mergeApiHeaders({}))
    .then(function(r){return r.ok?r.json():null;})
    .then(function(d){
      if(d&&d.avatarUrl){
        currentUserAvatarUrl=d.avatarUrl;
        // Ensure already-rendered messages pick up the persisted avatar on refresh.
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
  var u=getCurrentUser();
  userProfileWrap.style.display=(u==='lynn'||u==='raven')?'block':'none';
  if(u==='lynn'||u==='raven')loadUserProfile();
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
if(userSelect){
  try{ var u=getCurrentUser(); userSelect.value=u; lastSelectedUser=u; setClaudiaUserCookie(u); }catch(e){}
  fetchAvatarMe();
  loadAvatarPicker();
  showUserProfileSection();
  if(userProfileSave)userProfileSave.addEventListener('click',saveUserProfile);
  userSelect.addEventListener('change',function(){
    var next=userSelect.value;
    if(protectedUsers.indexOf(next)>=0&&signedInUser!==next){
      userSelect.value=lastSelectedUser;
      if(loginUser){loginUser.value=next;}
      if(loginError){loginError.textContent='Sign in as '+(userDisplayNames[next]||next)+' to access her chats.';loginError.style.display='block';}
      showLoginOverlay();
      return;
    }
    lastSelectedUser=next;
    try{ localStorage.setItem('claudia_user',next); }catch(e){}
    setClaudiaUserCookie(next);
    fetchAvatarMe();
    loadAvatarPicker();
    showUserProfileSection();
    loadAll().catch(function(){});
  });
}
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
})();
