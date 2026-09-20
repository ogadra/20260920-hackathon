let
  data = builtins.fromJSON (builtins.readFile /tmp/app-flake-archive.json);
  collect = value:
    if builtins.isAttrs value then
      (if value ? path then [ value.path ] else [ ])
      ++ builtins.concatLists (map collect (builtins.attrValues value))
    else if builtins.isList value then
      builtins.concatLists (map collect value)
    else
      [ ];
in
builtins.concatStringsSep "\n" (collect data)
