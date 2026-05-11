/* ── Tab bar (Chat only) ── */
var currentTab='chat';
function switchToTab(tab){
  currentTab=tab;
  if(hdrName)hdrName.textContent=currentConvoTitle||'Chat';
  if(hdrSub)hdrSub.textContent=currentSrc==='mobile'?'Claudia':(currentSrc||'');
  if(currentId)loadConvo(currentId,currentSrc);
  else{clearChat();showEmpty();}
  updateCopyConvoButtonVisibility();
}
if(tabBar){
  tabBar.querySelectorAll('.tab').forEach(function(btn){
    btn.addEventListener('click',function(){var t=btn.getAttribute('data-tab');if(t)switchToTab(t);});
  });
}
