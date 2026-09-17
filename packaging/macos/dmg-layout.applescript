on run scriptArguments
  set volumeFolder to POSIX file (item 1 of scriptArguments) as alias
  tell application "Finder"
    open volumeFolder
    set installerWindow to container window of volumeFolder
    set current view of installerWindow to icon view
    set toolbar visible of installerWindow to false
    set statusbar visible of installerWindow to false
    set pathbar visible of installerWindow to false
    set bounds of installerWindow to {200, 140, 800, 542}
    set layoutOptions to icon view options of installerWindow
    set arrangement of layoutOptions to not arranged
    set icon size of layoutOptions to 112
    set text size of layoutOptions to 14
    set shows item info of layoutOptions to false
    set shows icon preview of layoutOptions to true
    set label position of layoutOptions to bottom
    set background picture of layoutOptions to file ".background:background.png" of volumeFolder
    set position of item "Kaffeinate.app" of volumeFolder to {150, 190}
    set position of item "Applications" of volumeFolder to {450, 190}
    set position of item ".background" of volumeFolder to {900, 190}
    -- Finder saves toolbar and window changes asynchronously after reopening.
    close installerWindow
    open volumeFolder
    delay 1
    set installerWindow to container window of volumeFolder
    set bounds of installerWindow to {200, 140, 790, 532}
    delay 1
    set bounds of installerWindow to {200, 140, 800, 542}
    delay 3
    set position of item "Kaffeinate.app" of volumeFolder to {150, 190}
    set position of item "Applications" of volumeFolder to {450, 190}
    set position of item ".background" of volumeFolder to {900, 190}
    delay 1
    close installerWindow
  end tell
end run
