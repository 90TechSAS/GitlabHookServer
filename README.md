Knowed bugs :

- If the commit message includes a & character, the slack API send a 500 error (so the & is replaced with the word " and ")
- If the commit message includes a "" character, the slack API send a 500 error (so the " is replaced with two uniquotes : '')